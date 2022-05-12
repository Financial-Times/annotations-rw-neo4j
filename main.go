package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"
	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"

	logger "github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/http-handlers-go/v2/httphandlers"
	"github.com/Financial-Times/kafka-client-go/kafka"
	status "github.com/Financial-Times/service-status-go/httphandlers"

	"github.com/gorilla/mux"
	cli "github.com/jawher/mow.cli"
	metrics "github.com/rcrowley/go-metrics"
)

func main() {

	app := cli.App("annotations-rw", "A RESTful API for managing Annotations in neo4j")
	neoURL := app.String(cli.StringOpt{
		Name:   "neoUrl",
		Value:  "bolt://localhost:7687",
		Desc:   "neoURL must point to a leader node or to use neo4j:// scheme, otherwise writes will fail",
		EnvVar: "NEO_URL",
	})
	port := app.Int(cli.IntOpt{
		Name:   "port",
		Value:  8080,
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})
	logLevel := app.String(cli.StringOpt{
		Name:   "logLevel",
		Value:  "INFO",
		Desc:   "Logging level (DEBUG, INFO, WARN, ERROR)",
		EnvVar: "LOG_LEVEL",
	})
	dbDriverLogLevel := app.String(cli.StringOpt{
		Name:   "dbDriverLogLevel",
		Value:  "WARN",
		Desc:   "Db's driver logging level (DEBUG, INFO, WARN, ERROR)",
		EnvVar: "DB_DRIVER_LOG_LEVEL",
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

	appSystemCode := app.String(cli.StringOpt{
		Name:   "appSystemCode",
		Value:  "annotations-rw",
		Desc:   "Name of the service",
		EnvVar: "APP_SYSTEM_CODE",
	})

	app.Action = func() {
		logConf := logger.KeyNamesConfig{KeyTime: "@time"}
		log := logger.NewUPPLogger(*appName, *logLevel, logConf)
		log.WithFields(map[string]interface{}{"port": *port, "neoURL": *neoURL}).Infof("Service %s has successfully started.", *appName)

		dbLog := logger.NewUPPLogger(*appName+"-cmneo4j-driver", *dbDriverLogLevel)
		annotationsService, err := setupAnnotationsService(*neoURL, dbLog)
		if err != nil {
			log.WithError(err).Fatal("can't initialise annotations service")
		}
		healtcheckHandler := healthCheckHandler{
			appName:            *appName,
			systemCode:         *appSystemCode,
			annotationsService: annotationsService,
		}
		originMap, lifecycleMap, messageType, err := readConfigMap(*config)
		if err != nil {
			log.WithError(err).Fatal("can't read service configuration")
		}

		var f forwarder.QueueForwarder
		if *shouldForwardMessages {
			p, setupErr := setupMessageProducer(*brokerAddress, *producerTopic)
			if setupErr != nil {
				log.WithError(setupErr).Fatal("can't initialise message producer")
			}

			f = &forwarder.Forwarder{
				Producer:    p,
				MessageType: messageType,
			}
		}

		hh := httpHandler{
			annotationsService: annotationsService,
			forwarder:          f,
			originMap:          originMap,
			lifecycleMap:       lifecycleMap,
			messageType:        messageType,
			log:                log,
		}

		var qh queueHandler
		if *shouldConsumeMessages {
			var consumer kafka.Consumer
			consumer, err = setupMessageConsumer(*zookeeperAddress, *consumerGroup, *consumerTopic)
			if err != nil {
				log.WithError(err).Fatal("can't initialise message consumer")
			}
			healtcheckHandler.consumer = consumer

			qh = queueHandler{
				annotationsService: annotationsService,
				consumer:           consumer,
				forwarder:          f,
				originMap:          originMap,
				lifecycleMap:       lifecycleMap,
				messageType:        messageType,
				log:                log,
			}

			qh.Ingest()
		}

		http.Handle("/", router(&hh, &healtcheckHandler, log))

		go func() {
			err = startServer(*port)
			if err != nil {
				log.WithError(err).Fatal("http server error occurred")
			}
		}()

		waitForSignal()
		if *shouldConsumeMessages {
			log.Infof("Shutting down Kafka consumer")
			qh.consumer.Shutdown()
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("app could not start: %s", err)
		return
	}
}

func setupAnnotationsService(neoURL string, dbLogger *logger.UPPLogger) (annotations.Service, error) {
	driver, err := cmneo4j.NewDefaultDriver(neoURL, dbLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new cmneo4j driver: %v", err)
	}

	annotationsService := annotations.NewCypherAnnotationsService(driver)
	err = annotationsService.Initialise()
	if err != nil {
		return nil, fmt.Errorf("annotations service has not been initialised correctly: %w", err)
	}

	return annotationsService, nil
}

func setupMessageProducer(brokerAddress string, producerTopic string) (kafka.Producer, error) {
	producer, err := kafka.NewProducer(brokerAddress, producerTopic, kafka.DefaultProducerConfig())
	if err != nil {
		return nil, fmt.Errorf("cannot start queue producer: %w", err)
	}
	return producer, nil
}

func setupMessageConsumer(zookeeperAddress string, consumerGroup string, topic string) (kafka.Consumer, error) {
	// discard the output of zookeeper library
	noneLogger := logger.NewUPPInfoLogger("annotations-rw-neo4j-kafka-consumer")
	noneLogger.SetOutput(ioutil.Discard)
	groupConfig := kafka.DefaultConsumerConfig()
	groupConfig.Zookeeper.Logger = noneLogger

	config := kafka.Config{
		ZookeeperConnectionString: zookeeperAddress,
		ConsumerGroup:             consumerGroup,
		Topics:                    []string{topic},
		ConsumerGroupConfig:       groupConfig,
	}

	consumer, err := kafka.NewConsumer(config)
	if err != nil {
		return nil, fmt.Errorf("cannot start queue consumer: %w", err)
	}
	return consumer, nil
}

func readConfigMap(jsonPath string) (originMap map[string]string, lifecycleMap map[string]string, messageType string, err error) {

	file, err := ioutil.ReadFile(jsonPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("error reading configuration file: %w", err)
	}

	type config struct {
		OriginMap    map[string]string `json:"originMap"`
		LifecycleMap map[string]string `json:"lifecycleMap"`
		MessageType  string            `json:"messageType"`
	}
	var c config
	err = json.Unmarshal(file, &c)
	if err != nil {
		return nil, nil, "", fmt.Errorf("error marshalling config file: %w", err)
	}

	if c.MessageType == "" {
		return nil, nil, "", fmt.Errorf("message type is not configured: %w", errors.New("empty message type"))
	}

	return c.OriginMap, c.LifecycleMap, c.MessageType, nil
}

func router(hh *httpHandler, hc *healthCheckHandler, log *logger.UPPLogger) http.Handler {
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
	monitoringRouter = httphandlers.TransactionAwareRequestLoggingHandler(log, monitoringRouter)
	monitoringRouter = httphandlers.HTTPMetricsHandler(metrics.DefaultRegistry, monitoringRouter)

	return monitoringRouter
}

func startServer(port int) error {
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		return fmt.Errorf("unable to start server: %w", err)
	}
	return nil
}

func waitForSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}
