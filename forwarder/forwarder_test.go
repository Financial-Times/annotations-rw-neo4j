package forwarder_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"testing"
	"time"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	"github.com/Financial-Times/kafka-client-go/v3"
)

type InputMessage struct {
	Annotations annotations.Annotations `json:"annotations"`
	UUID        string                  `json:"uuid"`
}

const transactionID = "example-transaction-id"
const originSystem = "http://cmdb.ft.com/systems/pac"

func TestSendMessage(t *testing.T) {
	const expectedAnnotationsOutputBody = `{"payload":{"annotations":[{"thing":{"id":"http://api.ft.com/things/2cca9e2a-2248-3e48-abc1-93d718b91bbe","prefLabel":"China Politics \u0026 Policy","types":["http://www.ft.com/ontology/Topic"],"predicate":"majorMentions"},"provenances":[{"scores":[{"scoringSystem":"http://api.ft.com/scoringsystem/FT-RELEVANCE-SYSTEM","value":1},{"scoringSystem":"http://api.ft.com/scoringsystem/FT-CONFIDENCE-SYSTEM","value":1}]}]}],"lastModified":"%s","uuid":"3a636e78-5a47-11e7-9bc8-8055f264aa8b"},"contentUri":"http://pac.annotations-rw-neo4j.svc.ft.com/annotations/3a636e78-5a47-11e7-9bc8-8055f264aa8b","lastModified":"%[1]s"}`
	const expectedSuggestionsOutputBody = `{"payload":{"lastModified":"%s","suggestions":[{"thing":{"id":"http://api.ft.com/things/2cca9e2a-2248-3e48-abc1-93d718b91bbe","prefLabel":"China Politics \u0026 Policy","types":["http://www.ft.com/ontology/Topic"],"predicate":"majorMentions"},"provenances":[{"scores":[{"scoringSystem":"http://api.ft.com/scoringsystem/FT-RELEVANCE-SYSTEM","value":1},{"scoringSystem":"http://api.ft.com/scoringsystem/FT-CONFIDENCE-SYSTEM","value":1}]}]}],"uuid":"3a636e78-5a47-11e7-9bc8-8055f264aa8b"},"contentUri":"http://v2.suggestions-rw-neo4j.svc.ft.com/annotations/3a636e78-5a47-11e7-9bc8-8055f264aa8b","lastModified":"%[1]s"}`

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

			err = f.SendMessage(transactionID, originSystem, test.platformVersion, inputMessage.UUID, inputMessage.Annotations)
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
		})
	}
}

func TestCreateHeaders(t *testing.T) {
	headers := forwarder.CreateHeaders(transactionID, originSystem)

	checkHeaders := map[string]string{
		"X-Request-Id":     transactionID,
		"Origin-System-Id": originSystem,
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
