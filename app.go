package main

import (
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"encoding/json"
	"io/ioutil"
	"os/signal"
	"syscall"

	"github.com/Financial-Times/annotations-rw-neo4j/annotations"
	"github.com/Financial-Times/go-logger"
	"github.com/Financial-Times/http-handlers-go/httphandlers"
	"github.com/Financial-Times/kafka-client-go/kafka"
	"github.com/Financial-Times/neo-utils-go/neoutils"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"github.com/rcrowley/go-metrics"

	_ "github.com/joho/godotenv/autoload"
)

func main() {

	app := cli.App("annotations-rw", "A RESTful API for managing Annotations in neo4j")
	neoURL := app.String(cli.StringOpt{
		Name:   "neoUrl",
		Value:  "http://localhost:7474/db/data",
		Desc:   "neo4j endpoint URL",
		EnvVar: "NEO_URL",
	})
	port := app.Int(cli.IntOpt{
		Name:   "port",
		Value:  8080,
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})
	batchSize := app.Int(cli.IntOpt{
		Name:   "batchSize",
		Value:  1024,
		Desc:   "Maximum number of statements to execute per batch",
		EnvVar: "BATCH_SIZE",
	})
	logLevel := app.String(cli.StringOpt{
		Name:   "logLevel",
		Value:  "INFO",
		Desc:   "Logging level (DEBUG, INFO, WARN, ERROR)",
		EnvVar: "LOG_LEVEL",
	})
	config := app.String(cli.StringOpt{
		Name:   "lifecycleConfigPath",
		Value:  "annotation-config.json",
		Desc:   "Json Config file - containing two config maps: one for originHeader to lifecycle, another for lifecycle to platformVersion mappings. ",
		EnvVar: "LIFECYCLE_CONFIG_PATH",
	})
	zookeeperAddress := app.String(cli.StringOpt{
		Name:   "zookeeperAddress",
		Value:  "localhost:2181",
		Desc:   "Address of the zookeeper service",
		EnvVar: "ZOOKEEPER_ADDRESS",
	})
	shouldConsumeMessages := app.Bool(cli.BoolOpt{
		Name:   "shouldConsumeMessages",
		Value:  false,
		Desc:   "Boolean value specifying if this service should consume messages from the specified topic",
		EnvVar: "SHOULD_CONSUME_MESSAGES",
	})
	consumerGroup := app.String(cli.StringOpt{
		Name:   "consumerGroup",
		Desc:   "Kafka consumer group name",
		EnvVar: "CONSUMER_GROUP",
	})
	consumerTopic := app.String(cli.StringOpt{
		Name:   "consumerTopic",
		Desc:   "Kafka consumer topic name",
		EnvVar: "CONSUMER_TOPIC",
	})
	brokerAddress := app.String(cli.StringOpt{
		Name:   "brokerAddress",
		Value:  "localhost:9092",
		Desc:   "Kafka address",
		EnvVar: "BROKER_ADDRESS",
	})
	producerTopic := app.String(cli.StringOpt{
		Name:   "producerTopic",
		Value:  "PostPublicationMetadataEvents",
		Desc:   "Topic to which received messages will be forwarded",
		EnvVar: "PRODUCER_TOPIC",
	})
	shouldForwardMessages := app.Bool(cli.BoolOpt{
		Name:   "shouldForwardMessages",
		Value:  true,
		Desc:   "Decides if annotations messages should be forwarded to a post publication queue",
		EnvVar: "SHOULD_FORWARD_MESSAGES",
	})
	appName := app.String(cli.StringOpt{
		Name:   "appName",
		Value:  "annotations-rw",
		Desc:   "Name of the service",
		EnvVar: "APP_NAME",
	})

	app.Action = func() {
		logger.InitLogger(*appName, *logLevel)
		logger.WithFields(map[string]interface{}{"port": *port, "neoURL": *neoURL}).Infof("Service %s has successfully started.", *appName)

		annotationsService := setupAnnotationsService(*neoURL, *batchSize)
		healtcheckHandler := healthCheckHandler{annotationsService: annotationsService}
		originMap, lifecycleMap, messageType := readConfigMap(*config)

		httpHandler := httpHandler{annotationsService: annotationsService}
		httpHandler.originMap = originMap
		httpHandler.lifecycleMap = lifecycleMap
		httpHandler.messageType = messageType

		var p kafka.Producer
		if *shouldForwardMessages {
			p = setupMessageProducer(*brokerAddress, *producerTopic)
			httpHandler.producer = p
		}

		if *shouldConsumeMessages {
			consumer := setupMessageConsumer(*zookeeperAddress, *consumerGroup, *consumerTopic)
			healtcheckHandler.consumer = consumer

			qh := queueHandler{annotationsService: annotationsService, consumer: consumer, producer: p}
			qh.originMap = originMap
			qh.lifecycleMap = lifecycleMap
			qh.messageType = messageType
			qh.Ingest()

			go func() {
				waitForSignal()
				logger.Infof("Shutting down Kafka consumer")
				qh.consumer.Shutdown()
			}()
		}

		http.Handle("/", router(&httpHandler, &healtcheckHandler))
		startServer(*port)

	}
	app.Run(os.Args)
}

func setupAnnotationsService(neoURL string, bathSize int) annotations.Service {
	conf := neoutils.DefaultConnectionConfig()
	conf.BatchSize = bathSize
	db, err := neoutils.Connect(neoURL, conf)

	if err != nil {
		logger.WithError(err).Fatal("Error connecting to Neo4j")
	}

	annotationsService := annotations.NewCypherAnnotationsService(db)
	err = annotationsService.Initialise()
	if err != nil {
		logger.Errorf("annotations service has not been initalised correctly %s", err)
	}

	return annotations.NewCypherAnnotationsService(db)
}

func setupMessageProducer(brokerAddress string, producerTopic string) kafka.Producer {
	producer, err := kafka.NewProducer(brokerAddress, producerTopic, kafka.DefaultProducerConfig())
	if err != nil {
		logger.WithError(err).Fatal("Cannot start queue producer")
	}
	return producer
}

func setupMessageConsumer(zookeeperAddress string, consumerGroup string, topic string) kafka.Consumer {

	config := kafka.Config{
		ZookeeperConnectionString: zookeeperAddress,
		ConsumerGroup:             consumerGroup,
		Topics:                    []string{topic},
		ConsumerGroupConfig:       kafka.DefaultConsumerConfig()}

	consumer, err := kafka.NewConsumer(config)
	if err != nil {
		logger.WithError(err).Fatal("Cannot start queue consumer")
	}
	return consumer
}

func readConfigMap(jsonPath string) (originMap map[string]string, lifecycleMap map[string]string, messageType string) {

	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		logger.WithError(err).Fatal("Error reading configuration file")
	}

	type config struct {
		OriginMap    map[string]string `json:"originMap"`
		LifecycleMap map[string]string `json:"lifecycleMap"`
		MessageType  string            `json:"messageType"`
	}
	var c config
	err = json.Unmarshal(file, &c)
	if err != nil {
		logger.WithError(err).Fatal("Error marshalling config file")

	}

	if c.MessageType == "" {
		logger.WithError(errors.New("empty message type")).Fatal("Message type is not configured")
	}

	return c.OriginMap, c.LifecycleMap, c.MessageType
}

func router(hh *httpHandler, hc *healthCheckHandler) *mux.Router {
	servicesRouter := mux.NewRouter()
	servicesRouter.Headers("Content-type: application/json")

	// Then API specific ones:
	servicesRouter.HandleFunc("/content/{uuid}/annotations/{annotationLifecycle}", hh.GetAnnotations).Methods("GET")
	servicesRouter.HandleFunc("/content/{uuid}/annotations/{annotationLifecycle}", hh.PutAnnotations).Methods("PUT")
	servicesRouter.HandleFunc("/content/{uuid}/annotations/{annotationLifecycle}", hh.DeleteAnnotations).Methods("DELETE")
	servicesRouter.HandleFunc("/content/annotations/{annotationLifecycle}/__count", hh.CountAnnotations).Methods("GET")

	servicesRouter.HandleFunc("/__health", hc.Health()).Methods("GET")
	servicesRouter.HandleFunc("/__gtg", status.NewGoodToGoHandler(hc.GTG)).Methods("GET")
	servicesRouter.HandleFunc(status.PingPath, status.PingHandler).Methods("GET")
	servicesRouter.HandleFunc(status.PingPathDW, status.PingHandler).Methods("GET")
	servicesRouter.HandleFunc(status.BuildInfoPath, status.BuildInfoHandler).Methods("GET")
	servicesRouter.HandleFunc(status.BuildInfoPathDW, status.BuildInfoHandler).Methods("GET")

	var monitoringRouter http.Handler = servicesRouter
	monitoringRouter = httphandlers.TransactionAwareRequestLoggingHandler(logger.Logger(), monitoringRouter)
	monitoringRouter = httphandlers.HTTPMetricsHandler(metrics.DefaultRegistry, monitoringRouter)

	return servicesRouter
}

func startServer(port int) {
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		logger.WithError(err).Fatal("Unable to start server")
	}
}

func waitForSignal() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
