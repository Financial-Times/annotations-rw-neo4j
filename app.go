package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"encoding/json"
	"github.com/Financial-Times/annotations-rw-neo4j/annotations"
	"github.com/Financial-Times/base-ft-rw-app-go/baseftrwapp"
	"github.com/Financial-Times/http-handlers-go/httphandlers"
	"github.com/Financial-Times/kafka-client-go/kafka"
	"github.com/Financial-Times/neo-utils-go/neoutils"
	status "github.com/Financial-Times/service-status-go/httphandlers"
	log "github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/jawher/mow.cli"
	"github.com/rcrowley/go-metrics"
	"io/ioutil"
	"os/signal"
	"syscall"
)

func main() {

	app := cli.App("annotations-rw-neo4j", "A RESTful API for managing Annotations in neo4j")
	neoURL := app.String(cli.StringOpt{
		Name:   "neoUrl",
		Value:  "http://localhost:7474/db/data",
		Desc:   "neo4j endpoint URL",
		EnvVar: "NEO_URL",
	})
	graphiteTCPAddress := app.String(cli.StringOpt{
		Name:   "graphiteTCPAddress",
		Value:  "",
		Desc:   "Graphite TCP address, e.g. graphite.ft.com:2003. Leave as default if you do NOT want to output to graphite (e.g. if running locally",
		EnvVar: "GRAPHITE_ADDRESS",
	})
	graphitePrefix := app.String(cli.StringOpt{
		Name:   "graphitePrefix",
		Value:  "",
		Desc:   "Prefix to use. Should start with content, include the environment, and the host name. e.g. coco.pre-prod.roles-rw-neo4j.1 or content.test.people.rw.neo4j.ftaps58938-law1a-eu-t",
		EnvVar: "GRAPHITE_PREFIX",
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
	logMetrics := app.Bool(cli.BoolOpt{
		Name:   "logMetrics",
		Value:  false,
		Desc:   "Whether to log metrics. Set to true if running locally and you want metrics output",
		EnvVar: "LOG_METRICS",
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

	app.Action = func() {
		parsedLogLevel, err := log.ParseLevel(*logLevel)
		if err != nil {
			log.WithFields(log.Fields{"logLevel": logLevel, "err": err}).Fatal("Incorrect log level")
		}
		log.SetLevel(parsedLogLevel)
		log.Infof("annotations-rw-neo4j will listen on port: %d, connecting to: %s", *port, *neoURL)

		baseftrwapp.OutputMetricsIfRequired(*graphiteTCPAddress, *graphitePrefix, *logMetrics)

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
			qh.Ingest()

			go func() {
				waitForSignal()
				log.Info("Shutting down Kafka consumer")
				qh.consumer.Shutdown()
			}()
		}

		http.Handle("/", router(&httpHandler, &healtcheckHandler))
		startServer(*port)

	}
	log.Infof("Application started with args %s", os.Args)
	app.Run(os.Args)
}

func setupAnnotationsService(neoURL string, bathSize int) annotations.Service {
	conf := neoutils.DefaultConnectionConfig()
	conf.BatchSize = bathSize
	db, err := neoutils.Connect(neoURL, conf)

	if err != nil {
		log.Fatalf("Error connecting to neo4j %s", err)
	}
	annotationsService:=annotations.NewCypherAnnotationsService(db)
	 err = annotationsService.Initialise()
	if err!=nil{
		log.Errorf("annotations service has not been initalised correctly %s", err)
	}

	return annotationsService
}

func setupMessageProducer(brokerAddress string, producerTopic string) kafka.Producer {
	producer, err := kafka.NewProducer(brokerAddress, producerTopic)
	if err != nil {
		log.Fatal("Cannot start queue producer.")
	}
	return producer
}

func setupMessageConsumer(zookeeperAddress string, consumerGroup string, topic string) kafka.Consumer {
	consumer, err := kafka.NewConsumer(zookeeperAddress, consumerGroup, []string{topic}, kafka.DefaultConsumerConfig())
	if err != nil {
		log.Fatal("Cannot start queue consumer")
	}
	return consumer
}

func readConfigMap(jsonPath string) (originMap map[string]string, lifecycleMap map[string]string, messageType string) {

	file, e := ioutil.ReadFile(jsonPath)
	if e != nil {
		log.Fatal("Error reading config file", e)
	}

	type config struct {
		OriginMap    map[string]string `json:"originMap"`
		LifecycleMap map[string]string `json:"lifecycleMap"`
		MessageType  string            `json:"messageType"`
	}
	var c config
	e = json.Unmarshal(file, &c)
	if e != nil {
		log.Fatal("Error marshalling config file", e)
	}

	if c.MessageType == "" {
		log.Fatal("Message type is not configured.")
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
	monitoringRouter = httphandlers.TransactionAwareRequestLoggingHandler(log.StandardLogger(), monitoringRouter)
	monitoringRouter = httphandlers.HTTPMetricsHandler(metrics.DefaultRegistry, monitoringRouter)

	return servicesRouter
}

func startServer(port int) {
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Fatalf("Unable to start server: %v", err)
	}
}

func waitForSignal() {
	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
