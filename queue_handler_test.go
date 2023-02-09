package main

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/Financial-Times/kafka-client-go/v3"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	logger "github.com/Financial-Times/go-logger/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type QueueHandlerTestSuite struct {
	suite.Suite
	headers            map[string]string
	body               []byte
	message            kafka.FTMessage
	queueMessage       queueMessage
	annotationsService *mockAnnotationsService
	forwarder          *mockForwarder
	originMap          map[string]string
	lifecycleMap       map[string]string
	tid                string
	originSystem       string
	bookmark           string
	messageType        string
	log                *logger.UPPLogger
}

func (suite *QueueHandlerTestSuite) SetupTest() {
	var err error
	suite.log = logger.NewUPPInfoLogger("annotations-rw")
	suite.tid = "tid_sample"
	suite.originSystem = "http://cmdb.ft.com/systems/pac"
	suite.bookmark = "FB:kcwQnrEEnFpfSJ2PtiykK/JNh8oBozhIkA=="
	suite.forwarder = new(mockForwarder)
	suite.headers = forwarder.CreateHeaders(suite.tid, suite.originSystem, suite.bookmark)
	suite.body, err = ioutil.ReadFile("exampleAnnotationsMessage.json")
	assert.NoError(suite.T(), err, "Unexpected error")
	suite.message = kafka.NewFTMessage(suite.headers, string(suite.body))
	err = json.Unmarshal(suite.body, &suite.queueMessage)
	assert.NoError(suite.T(), err, "Unexpected error")
	suite.annotationsService = new(mockAnnotationsService)

	suite.originMap, suite.lifecycleMap, suite.messageType, err = readConfigMap("annotation-config.json")
	assert.NoError(suite.T(), err, "Unexpected config error")
}

func TestQueueHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(QueueHandlerTestSuite))
}

func (suite *QueueHandlerTestSuite) TestQueueHandler_Ingest() {
	suite.annotationsService.On("Write", suite.queueMessage.UUID, annotationLifecycle, platformVersion, suite.queueMessage.Annotations).Return(suite.bookmark, nil)
	suite.forwarder.On("SendMessage", suite.tid, suite.originSystem, suite.bookmark, platformVersion, suite.queueMessage.UUID, suite.queueMessage.Annotations).Return(nil)

	qh := &queueHandler{
		annotationsService: suite.annotationsService,
		consumer:           mockConsumer{message: suite.message},
		forwarder:          suite.forwarder,
		originMap:          suite.originMap,
		lifecycleMap:       suite.lifecycleMap,
		messageType:        suite.messageType,
		log:                suite.log,
	}
	qh.Ingest()

	suite.annotationsService.AssertCalled(suite.T(), "Write", suite.queueMessage.UUID, annotationLifecycle, platformVersion, suite.queueMessage.Annotations)
	suite.forwarder.AssertCalled(suite.T(), "SendMessage", suite.tid, suite.originSystem, suite.bookmark, platformVersion, suite.queueMessage.UUID, suite.queueMessage.Annotations)
}

func (suite *QueueHandlerTestSuite) TestQueueHandler_Ingest_ProducerNil() {
	suite.annotationsService.On("Write", suite.queueMessage.UUID, annotationLifecycle, platformVersion, suite.queueMessage.Annotations).Return(suite.bookmark, nil)

	qh := queueHandler{
		annotationsService: suite.annotationsService,
		consumer:           mockConsumer{message: suite.message},
		forwarder:          nil,
		originMap:          suite.originMap,
		lifecycleMap:       suite.lifecycleMap,
		messageType:        suite.messageType,
		log:                suite.log,
	}
	qh.Ingest()

	suite.annotationsService.AssertCalled(suite.T(), "Write", suite.queueMessage.UUID, annotationLifecycle, platformVersion, suite.queueMessage.Annotations)
	suite.forwarder.AssertNumberOfCalls(suite.T(), "SendMessage", 0)
}

func (suite *QueueHandlerTestSuite) TestQueueHandler_Ingest_JsonError() {
	body := "invalid json"
	message := kafka.NewFTMessage(suite.headers, string(body))

	qh := &queueHandler{
		annotationsService: suite.annotationsService,
		consumer:           mockConsumer{message: message},
		forwarder:          suite.forwarder,
		originMap:          suite.originMap,
		lifecycleMap:       suite.lifecycleMap,
		log:                suite.log,
	}
	qh.Ingest()

	suite.forwarder.AssertNumberOfCalls(suite.T(), "SendMessage", 0)
	suite.annotationsService.AssertNumberOfCalls(suite.T(), "Write", 0)
}

func (suite *QueueHandlerTestSuite) TestQueueHandler_Ingest_InvalidOrigin() {
	suite.headers["Origin-System-Id"] = "http://cmdb.ft.com/systems/invalidOrigin"
	message := kafka.NewFTMessage(suite.headers, string(suite.body))

	qh := &queueHandler{
		annotationsService: suite.annotationsService,
		consumer:           mockConsumer{message: message},
		forwarder:          suite.forwarder,
		originMap:          suite.originMap,
		lifecycleMap:       suite.lifecycleMap,
		log:                suite.log,
	}
	qh.Ingest()

	// if message is valid, the first method to be called is annotationsService.Write
	suite.annotationsService.AssertNumberOfCalls(suite.T(), "Write", 0)
}
