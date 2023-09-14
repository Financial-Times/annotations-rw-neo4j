package forwarder

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/Financial-Times/kafka-client-go/v3"

	"github.com/google/uuid"
)

// The outputMessage represents the structure of the JSON object that is written in the body of the message
// sent to Kafka by the SendMessage method of Forwarder.
//
// The Payload property needs to be a "map[string]interface{}" because one of its subproperties
// has variable name which has to be the same as the MessageType of the Forwarder.
//
// Please note that the used encoding/json package marshals maps in sorted key order and structs in the order that the fields are declared.
type outputMessage struct {
	Payload      map[string]interface{} `json:"payload"`
	ContentURI   string                 `json:"contentUri"`
	LastModified string                 `json:"lastModified"`
}

// QueueForwarder is the interface implemented by types that can send annotation messages to a queue.
type QueueForwarder interface {
	SendMessage(transactionID string, originSystem string, bookmark string, platformVersion string, uuid string, annotations interface{}) error
}

type kafkaProducer interface {
	SendMessage(message kafka.FTMessage) error
}

// A Forwarder facilitates sending a message to Kafka via kafka.Producer.
type Forwarder struct {
	Producer    kafkaProducer
	MessageType string
}

// SendMessage marshals an annotations payload using the outputMessage format and sends it to a Kafka.
func (f Forwarder) SendMessage(transactionID string, originSystem string, bookmark string, platformVersion string, uuid string, annotations interface{}) error {
	headers := CreateHeaders(transactionID, originSystem, bookmark)
	body, err := f.prepareBody(platformVersion, uuid, annotations, headers["Message-Timestamp"])
	if err != nil {
		return err
	}

	return f.Producer.SendMessage(kafka.NewFTMessage(headers, body))
}

func (f Forwarder) prepareBody(platformVersion string, uuid string, anns interface{}, lastModified string) (string, error) {
	wrappedMsg := outputMessage{
		Payload: map[string]interface{}{
			strings.ToLower(f.MessageType): anns,
			"lastModified":                 lastModified,
			"uuid":                         uuid,
		},
		ContentURI:   "http://" + platformVersion + "." + strings.ToLower(f.MessageType) + "-rw-neo4j.svc.ft.com/annotations/" + uuid,
		LastModified: lastModified,
	}

	// Given the type of data we are marshalling, there is no possible input that can trigger an error here
	// but we are handling errors just to be principled
	res, err := json.Marshal(wrappedMsg)
	if err != nil {
		return "", err
	}

	return string(res), nil
}

// CreateHeaders returns the relevant map with all the necessary kafka.FTMessage headers
// according to the specified transaction ID and origin system.
func CreateHeaders(transactionID string, originSystem string, bookmark string) map[string]string {
	const dateFormat = "2006-01-02T15:04:05.000Z0700"
	messageUUID := uuid.New()
	return map[string]string{
		"X-Request-Id":      transactionID,
		"Message-Timestamp": time.Now().Format(dateFormat),
		"Message-Id":        messageUUID.String(),
		"Message-Type":      "concept-annotation",
		"Content-Type":      "application/json",
		"Origin-System-Id":  originSystem,
		"Neo4j-Bookmark":    bookmark,
	}
}
