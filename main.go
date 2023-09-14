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

	"github.com/Financial-Times/cm-annotations-ontology/validator"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"
	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"
	"github.com/Financial-Times/kafka-client-go/v3"

	logger "github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/http-handlers-go/v2/httphandlers"
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
	consumerTopics := app.Strings(cli.StringsOpt{
		Name:   "consumerTopics",
		Desc:   "Kafka consumer topics",
		EnvVar: "CONSUMER_TOPICS",
	})
	kafkaLagTolerance := app.Int(cli.IntOpt{
		Name:   "kafkaLagTolerance",
		Desc:   "Kafka consumer lag tolerance",
		EnvVar: "KAFKA_LAG_TOLERANCE",
	})
	kafkaAddress := app.String(cli.StringOpt{
		Name:   "kafkaAddress",
		Value:  "kafka:9092",
		Desc:   "Kafka address",
		EnvVar: "KAFKA_ADDRESS",
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
	publicAPIHost := app.String(cli.StringOpt{
		Name:   "apiURL",
		Desc:   "API Gateway URL used when building the thing ID url in the response, in the format scheme://host",
		EnvVar: "API_HOST",
	})

	app.Action = func() {
		logConf := logger.KeyNamesConfig{KeyTime: "@time"}
		log := logger.NewUPPLogger(*appName, *logLevel, logConf)
		log.WithFields(map[string]interface{}{"port": *port, "neoURL": *neoURL}).Infof("Service %s has successfully started.", *appName)

		dbLog := logger.NewUPPLogger(*appName+"-cmneo4j-driver", *dbDriverLogLevel)
		annotationsService, err := setupAnnotationsService(*neoURL, *publicAPIHost, dbLog)
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
			p := setupMessageProducer(*kafkaAddress, *producerTopic, log)

			f = &forwarder.Forwarder{
				Producer:    p,
				MessageType: messageType,
			}
		}

		validator := validator.NewSchemaValidator(log)

		hh := httpHandler{
			validator:          validator.GetJSONValidator(),
			annotationsService: annotationsService,
			forwarder:          f,
			originMap:          originMap,
			lifecycleMap:       lifecycleMap,
			messageType:        messageType,
			log:                log,
		}

		var qh queueHandler
		if *shouldConsumeMessages {
			consumer := setupMessageConsumer(*kafkaAddress, *consumerGroup, *consumerTopics, int64(*kafkaLagTolerance), log)

			healtcheckHandler.consumer = consumer

			qh = queueHandler{
				validator:          validator.GetJSONValidator(),
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
			qh.consumer.Close()
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("app could not start: %s", err)
		return
	}
}

func setupAnnotationsService(neoURL, publicAPIURL string, dbLogger *logger.UPPLogger) (annotations.Service, error) {
	driver, err := cmneo4j.NewDefaultDriver(neoURL, dbLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new cmneo4j driver: %v", err)
	}

	annotationsService, err := annotations.NewCypherAnnotationsService(driver, publicAPIURL)
	if err != nil {
		return nil, fmt.Errorf("creating annotations service: %w", err)
	}

	err = annotationsService.Initialise()
	if err != nil {
		return nil, fmt.Errorf("annotations service has not been initialised correctly: %w", err)
	}

	return annotationsService, nil
}

func setupMessageProducer(brokerAddress string, producerTopic string, log *logger.UPPLogger) *kafka.Producer {
	producerConfig := kafka.ProducerConfig{
		BrokersConnectionString: brokerAddress,
		Topic:                   producerTopic,
		ConnectionRetryInterval: 0,
		Options:                 kafka.DefaultProducerOptions(),
	}

	producer := kafka.NewProducer(producerConfig, log)

	return producer
}

func setupMessageConsumer(kafkaAddress string, consumerGroup string, topics []string, lagTolerance int64, log *logger.UPPLogger) *kafka.Consumer {
	consumerConfig := kafka.ConsumerConfig{
		BrokersConnectionString: kafkaAddress,
		ConsumerGroup:           consumerGroup,
		Options:                 kafka.DefaultConsumerOptions(),
	}

	var kafkaTopics []*kafka.Topic
	for _, topic := range topics {
		kafkaTopics = append(kafkaTopics, kafka.NewTopic(topic, kafka.WithLagTolerance(lagTolerance)))
	}

	return kafka.NewConsumer(consumerConfig, kafkaTopics, log)
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
