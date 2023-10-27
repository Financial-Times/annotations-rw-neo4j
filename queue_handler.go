package main

import (
	"encoding/json"

	"github.com/Financial-Times/kafka-client-go/v3"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	logger "github.com/Financial-Times/go-logger/v2"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"

	"github.com/pkg/errors"
)

const (
	nextVideoOrigin   = "http://cmdb.ft.com/systems/next-video-editor"
	cmsMessageType    = "cms-content-published"
	suggestionsMsgKey = "suggestions"
	annotationsMsgKey = "annotations"
	uuidMsgKey        = "uuid"
	publicationMsgKey = "publication"
)

type kafkaConsumer interface {
	Start(func(message kafka.FTMessage))
	Close() error
	MonitorCheck() error
	ConnectivityCheck() error
}

type jsonValidator interface {
	Validate(interface{}) error
}

type queueHandler struct {
	validator          jsonValidator
	annotationsService annotations.Service
	consumer           kafkaConsumer
	forwarder          forwarder.QueueForwarder
	originMap          map[string]string
	lifecycleMap       map[string]string
	messageType        string
	log                *logger.UPPLogger
}

func (qh *queueHandler) Ingest() {
	qh.consumer.Start(func(message kafka.FTMessage) {
		tid, found := message.Headers[transactionidutils.TransactionIDHeader]
		if !found {
			qh.log.Error("Missing transaction id from message")
			return
		}

		originSystem, found := message.Headers["Origin-System-Id"]
		if !found {
			qh.log.Error("Missing Origin-System-Id header from message")
			return
		}

		// Ignoring Video messages from NativeCmsMetadataPublicationEvents topic as the corresponding ones produced from
		// the upp-next-video-annotations-mapper would be ingested from the ConceptAnnotations topic
		if originSystem == nextVideoOrigin && message.Headers["Message-Type"] == cmsMessageType {
			qh.log.WithField("Message-Type", cmsMessageType).WithField("Origin-System-Id", nextVideoOrigin).Info("Ignoring message")
			return
		}

		lifecycle, platformVersion, err := qh.getSourceFromHeader(originSystem)
		if err != nil {
			qh.log.WithError(err).Error("Could not get source from header")
			return
		}

		var annMsg map[string]interface{}
		err = json.Unmarshal([]byte(message.Body), &annMsg)
		if err != nil {
			qh.log.WithTransactionID(tid).Error("Cannot process received message", tid)
			return
		}

		contentUUID := annMsg[uuidMsgKey].(string)
		var publication []interface{}
		pubSlice, ok := annMsg[publicationMsgKey]
		if ok {
			publication, ok = pubSlice.([]interface{})
			if !ok {
				qh.log.Error("Publication field format is not supported")
				return
			}
		}

		var bookmark string
		if qh.messageType == "Annotations" {
			err = qh.validate(annMsg[annotationsMsgKey])
			if err != nil {
				qh.log.WithError(err).Error("Validation error")
				return
			}
			bookmark, err = qh.annotationsService.Write(contentUUID, lifecycle, platformVersion, publication, annMsg[annotationsMsgKey])
		} else {
			err = qh.validate(annMsg[suggestionsMsgKey])
			if err != nil {
				qh.log.WithError(err).Error("Validation error")
				return
			}
			bookmark, err = qh.annotationsService.Write(contentUUID, lifecycle, platformVersion, publication, annMsg[suggestionsMsgKey])
		}

		if err != nil {
			qh.log.WithMonitoringEvent("SaveNeo4j", tid, qh.messageType).WithUUID(contentUUID).WithError(err).Error("Cannot write to Neo4j")
			return
		}

		qh.log.WithMonitoringEvent("SaveNeo4j", tid, qh.messageType).WithUUID(contentUUID).Infof("%s successfully written in Neo4j", qh.messageType)

		//forward message to the next queue
		if qh.forwarder != nil {
			qh.log.WithTransactionID(tid).WithUUID(contentUUID).Debug("Forwarding message to the next queue")
			err := qh.forwarder.SendMessage(tid, originSystem, bookmark, platformVersion, contentUUID, annMsg[annotationsMsgKey])
			if err != nil {
				qh.log.WithError(err).WithUUID(contentUUID).WithTransactionID(tid).Error("Could not forward a message to kafka")
				return
			}
			return
		}
	})
}

func (qh *queueHandler) getSourceFromHeader(originSystem string) (string, string, error) {
	annotationLifecycle, found := qh.originMap[originSystem]
	if !found {
		return "", "", errors.Errorf("Annotation Lifecycle not found for origin system id: %s", originSystem)
	}

	platformVersion, found := qh.lifecycleMap[annotationLifecycle]
	if !found {
		return "", "", errors.Errorf("Platform version not found for origin system id: %s and annotation lifecycle: %s", originSystem, annotationLifecycle)
	}
	return annotationLifecycle, platformVersion, nil
}

func (qh *queueHandler) validate(annotations interface{}) error {
	for _, annotation := range annotations.([]interface{}) {
		err := qh.validator.Validate(annotation)
		if err != nil {
			return err
		}
	}
	return nil
}
