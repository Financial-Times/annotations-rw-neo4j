package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Financial-Times/cm-annotations-ontology/validator"

	logger "github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/kafka-client-go/v3"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

const (
	knownUUID           = "12345"
	annotationLifecycle = "annotations-pac"
	platformVersion     = "pac"
	bookmark            = "bookmark"
)

type HttpHandlerTestSuite struct {
	suite.Suite
	body               []byte
	annotations        []interface{}
	annotationsService *mockAnnotationsService
	forwarder          *mockForwarder
	message            kafka.FTMessage
	healthCheckHandler healthCheckHandler
	originMap          map[string]string
	lifecycleMap       map[string]string
	tid                string
	messageType        string
	log                *logger.UPPLogger
	validator          jsonValidator
}

func (suite *HttpHandlerTestSuite) SetupTest() {
	os.Setenv("JSON_SCHEMAS_PATH", "./schemas")
	os.Setenv("JSON_SCHEMA_NAME", "annotations-pac.json;annotations-next-video.json;annotations-v2.json;annotations-sv.json")

	suite.log = logger.NewUPPInfoLogger("annotations-rw")
	var err error
	suite.body, err = ioutil.ReadFile("examplePutBody.json")
	assert.NoError(suite.T(), err, "Unexpected error")

	suite.annotations, err = decode(bytes.NewReader(suite.body))
	assert.NoError(suite.T(), err, "Unexpected error")

	suite.annotationsService = new(mockAnnotationsService)
	suite.forwarder = new(mockForwarder)
	suite.tid = "tid_sample"

	suite.healthCheckHandler = healthCheckHandler{}
	suite.originMap, suite.lifecycleMap, suite.messageType, err = readConfigMap("annotation-config.json")
	suite.validator = validator.NewSchemaValidator(suite.log).GetJSONValidator()

	assert.NoError(suite.T(), err, "Unexpected error")
}

func TestHttpHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HttpHandlerTestSuite))
}

func (suite *HttpHandlerTestSuite) TestPutHandler_Success() {
	suite.annotationsService.On("Write", knownUUID, annotationLifecycle, platformVersion, []interface{}{}, suite.annotations).Return(bookmark, nil)
	suite.forwarder.On("SendMessage", suite.tid, "http://cmdb.ft.com/systems/pac", bookmark, platformVersion, knownUUID, suite.annotations).Return(nil).Once()
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", suite.body)
	request.Header.Add("X-Request-Id", suite.tid)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusCreated == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusCreated))
	assert.JSONEq(suite.T(), message("Annotations for content 12345 created"), rec.Body.String(), "Wrong body")
	suite.forwarder.AssertExpectations(suite.T())
}

func (suite *HttpHandlerTestSuite) TestPutHandler_ParseError() {
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", []byte(`{"id": "1234"}`))
	request.Header.Add("X-Request-Id", suite.tid)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusBadRequest == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusBadRequest))
}

func (suite *HttpHandlerTestSuite) TestPutHandler_ValidationError() {
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", []byte(`"{"thing": {"prefLabel": "Apple"}`))
	request.Header.Add("X-Request-Id", suite.tid)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusBadRequest == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusBadRequest))
}

func (suite *HttpHandlerTestSuite) TestPutHandler_NotJson() {
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "text/html", suite.body)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusBadRequest == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusBadRequest))
}

func (suite *HttpHandlerTestSuite) TestPutHandler_WriteFailed() {
	suite.annotationsService.On("Write", knownUUID, annotationLifecycle, platformVersion, []interface{}{}, suite.annotations).Return("", errors.New("Write failed"))
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", suite.body)
	request.Header.Add("X-Request-Id", suite.tid)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusServiceUnavailable == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusServiceUnavailable))
}

func (suite *HttpHandlerTestSuite) TestPutHandler_ForwardingFailed() {
	suite.annotationsService.On("Write", knownUUID, annotationLifecycle, platformVersion, []interface{}{}, suite.annotations).Return(bookmark, nil)
	suite.forwarder.On("SendMessage", suite.tid, "http://cmdb.ft.com/systems/pac", bookmark, platformVersion, knownUUID, suite.annotations).Return(errors.New("forwarding failed"))
	request := newRequest("PUT", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", suite.body)
	request.Header.Add("X-Request-Id", suite.tid)
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	rec := httptest.NewRecorder()
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusInternalServerError == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusInternalServerError))
	suite.forwarder.AssertExpectations(suite.T())
}

func (suite *HttpHandlerTestSuite) TestGetHandler_Success() {
	suite.annotationsService.On("Read", knownUUID, mock.Anything, annotationLifecycle).Return(suite.annotations, true, nil)
	request := newRequest("GET", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusOK == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusOK))
	expectedResponse, err := json.Marshal(suite.annotations)
	assert.NoError(suite.T(), err, "")
	assert.JSONEq(suite.T(), string(expectedResponse), rec.Body.String(), "Wrong body")
}

func (suite *HttpHandlerTestSuite) TestGetHandler_NotFound() {
	suite.annotationsService.On("Read", knownUUID, mock.Anything, annotationLifecycle).Return(nil, false, nil)
	request := newRequest("GET", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusNotFound == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusNotFound))
}

func (suite *HttpHandlerTestSuite) TestGetHandler_ReadError() {
	suite.annotationsService.On("Read", knownUUID, mock.Anything, annotationLifecycle).Return(nil, false, errors.New("Read error"))
	request := newRequest("GET", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusServiceUnavailable == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusServiceUnavailable))
}

func (suite *HttpHandlerTestSuite) TestGetHandler_InvalidLifecycle() {
	request := newRequest("GET", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, "annotations-invalid"), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusBadRequest == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusBadRequest))
}

func (suite *HttpHandlerTestSuite) TestDeleteHandler_Success() {
	suite.annotationsService.On("Delete", knownUUID, annotationLifecycle).Return(true, bookmark, nil)
	request := newRequest("DELETE", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusNoContent == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusNoContent))
}

func (suite *HttpHandlerTestSuite) TestDeleteHandler_NotFound() {
	suite.annotationsService.On("Delete", knownUUID, annotationLifecycle).Return(false, bookmark, nil)
	request := newRequest("DELETE", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusNotFound == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusNotFound))
}

func (suite *HttpHandlerTestSuite) TestDeleteHandler_DeleteError() {
	suite.annotationsService.On("Delete", knownUUID, annotationLifecycle).Return(false, bookmark, errors.New("Delete error"))
	request := newRequest("DELETE", fmt.Sprintf("/content/%s/annotations/%s", knownUUID, annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusServiceUnavailable == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusServiceUnavailable))
}

func (suite *HttpHandlerTestSuite) TestCount_Success() {
	suite.annotationsService.On("Count", annotationLifecycle, mock.Anything, platformVersion).Return(10, nil)
	request := newRequest("GET", fmt.Sprintf("/content/annotations/%s/__count", annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	router(&httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusOK == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusOK))
}

func (suite *HttpHandlerTestSuite) TestCount_CountError() {
	suite.annotationsService.On("Count", annotationLifecycle, mock.Anything, platformVersion).Return(0, errors.New("Count error"))
	request := newRequest("GET", fmt.Sprintf("/content/annotations/%s/__count", annotationLifecycle), "application/json", nil)
	rec := httptest.NewRecorder()
	handler := httpHandler{suite.validator, suite.annotationsService, suite.forwarder, suite.originMap, suite.lifecycleMap, suite.messageType, suite.log}
	router(&handler, &suite.healthCheckHandler, suite.log).ServeHTTP(rec, request)
	assert.True(suite.T(), http.StatusServiceUnavailable == rec.Code, fmt.Sprintf("Wrong response code, was %d, should be %d", rec.Code, http.StatusServiceUnavailable))
}

func newRequest(method, url, contentType string, body []byte) *http.Request {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
	if err != nil {
		panic(err)
	}
	req.Header.Add("Content-Type", contentType)
	return req
}

func message(errMsg string) string {
	return fmt.Sprintf("{\"message\": \"%s\"}\n", errMsg)
}
