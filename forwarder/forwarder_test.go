package forwarder_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/ontology"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	"github.com/Financial-Times/kafka-client-go/v3"
)

type InputMessage struct {
	Annotations ontology.Annotations `json:"annotations"`
	UUID        string               `json:"uuid"`
}

const transactionID = "example-transaction-id"
const originSystem = "http://cmdb.ft.com/systems/pac"
const bookmark = "FB:kcwQnrEEnFpfSJ2PtiykK/JNh8oBozhIkA=="

func TestSendMessage(t *testing.T) {
	const expectedAnnotationsOutputBody = `{"payload":{"annotations":[{"id":"http://api.ft.com/things/2384fa7a-d514-3d6a-a0ea-3a711f66d0d8","prefLabel":"Apple","types":["http://www.ft.com/ontology/organisation/Organisation","http://www.ft.com/ontology/core/Thing","http://www.ft.com/ontology/concept/Concept"],"predicate":"mentions","relevanceScore":1,"confidenceScore":0.9932743203464962,"annotatedBy":"http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a","annotatedDate":"2016-01-20T19:43:47.314Z","annotatedDateEpoch":1453180597}],"lastModified":"%s","uuid":"3a636e78-5a47-11e7-9bc8-8055f264aa8b"},"contentUri":"http://pac.annotations-rw-neo4j.svc.ft.com/annotations/3a636e78-5a47-11e7-9bc8-8055f264aa8b","lastModified":"%[1]s"}`
	const expectedSuggestionsOutputBody = `{"payload":{"lastModified":"%s","suggestions":[{"id":"http://api.ft.com/things/2384fa7a-d514-3d6a-a0ea-3a711f66d0d8","prefLabel":"Apple","types":["http://www.ft.com/ontology/organisation/Organisation","http://www.ft.com/ontology/core/Thing","http://www.ft.com/ontology/concept/Concept"],"predicate":"mentions","relevanceScore":1,"confidenceScore":0.9932743203464962,"annotatedBy":"http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a","annotatedDate":"2016-01-20T19:43:47.314Z","annotatedDateEpoch":1453180597}],"uuid":"3a636e78-5a47-11e7-9bc8-8055f264aa8b"},"contentUri":"http://v2.suggestions-rw-neo4j.svc.ft.com/annotations/3a636e78-5a47-11e7-9bc8-8055f264aa8b","lastModified":"%[1]s"}`

	body, err := ioutil.ReadFile("../exampleAnnotationsMessage.json")
	if err != nil {
		t.Fatal("Unexpected error reading example message")
	}
	inputMessage := InputMessage{}
	err = json.Unmarshal(body, &inputMessage)
	if err != nil {
		t.Fatal("Unexpected error unmarshalling example message")
	}
	tests := []struct {
		name            string
		messageType     string
		platformVersion string
		expectedBody    string
	}{
		{
			name:            "Annotations Message",
			messageType:     "Annotations",
			platformVersion: "pac",
			expectedBody:    expectedAnnotationsOutputBody,
		},
		{
			name:            "Suggestions Message",
			messageType:     "Suggestions",
			platformVersion: "v2",
			expectedBody:    expectedSuggestionsOutputBody,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := new(mockProducer)
			f := forwarder.Forwarder{
				Producer:    p,
				MessageType: test.messageType,
			}

			err = f.SendMessage(transactionID, originSystem, bookmark, test.platformVersion, inputMessage.UUID, inputMessage.Annotations)
			if err != nil {
				t.Error("Error sending message")
			}

			res := p.getLastMessage()
			if res.Body != fmt.Sprintf(test.expectedBody, res.Headers["Message-Timestamp"]) {
				t.Errorf("Unexpected Kafka message processed, expected: \n`%s`\n\n but recevied: \n`%s`", test.expectedBody, res.Body)
			}
			if res.Headers["X-Request-Id"] != transactionID {
				t.Errorf("Unexpected Kafka X-Request-Id, expected `%s` but recevied `%s`", transactionID, res.Headers["X-Request-Id"])
			}
			if res.Headers["Origin-System-Id"] != originSystem {
				t.Errorf("Unexpected Kafka Origin-System-Id, expected `%s` but recevied `%s`", originSystem, res.Headers["Origin-System-Id"])
			}
			if res.Headers["Neo4j-Bookmark"] != bookmark {
				t.Errorf("Unexpected Kafka Neo4j-Bookmark, expected `%s` but recevied `%s`", bookmark, res.Headers["Neo4j-Bookmark"])
			}
		})
	}
}

func TestCreateHeaders(t *testing.T) {
	headers := forwarder.CreateHeaders(transactionID, originSystem, bookmark)

	checkHeaders := map[string]string{
		"X-Request-Id":     transactionID,
		"Origin-System-Id": originSystem,
		"Neo4j-Bookmark":   bookmark,
		"Message-Type":     "concept-annotation",
		"Content-Type":     "application/json",
	}
	for k, v := range checkHeaders {
		if headers[k] != v {
			t.Errorf("Unexpected %s, expected `%s` but recevied `%s`", k, v, headers[k])
		}
	}

	const dateFormat = "2006-01-02T15:04:05.000Z0700"
	if _, err := time.Parse(dateFormat, headers["Message-Timestamp"]); err != nil {
		t.Errorf("Unexpected Message-Timestamp format, expected `%s` but recevied `%s`", dateFormat, headers["Message-Timestamp"])
	}
	r := regexp.MustCompile("^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[8|9|a|b][a-f0-9]{3}-[a-f0-9]{12}$")
	if !r.MatchString(headers["Message-Id"]) {
		t.Errorf("Unexpected Content-Type, expected UUID v4 but recevied `%s`", headers["Message-Id"])
	}
}

type mockProducer struct {
	message kafka.FTMessage
}

func (mp *mockProducer) SendMessage(message kafka.FTMessage) error {
	mp.message = message
	return nil
}

func (mp *mockProducer) getLastMessage() kafka.FTMessage {
	return mp.message
}

func (mp *mockProducer) ConnectivityCheck() error {
	return nil
}

func (mp *mockProducer) Shutdown() {
}
