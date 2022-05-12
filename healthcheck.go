package main

import (
	"net/http"
	"time"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/kafka-client-go/kafka"
	"github.com/Financial-Times/service-status-go/gtg"
)

type healthCheckHandler struct {
	systemCode         string
	appName            string
	annotationsService annotations.Service
	consumer           kafka.Consumer
}

func (h healthCheckHandler) Health() func(w http.ResponseWriter, r *http.Request) {
	checks := []fthealth.Check{h.writerCheck()}
	if h.consumer != nil {
		checks = append(checks, h.readQueueCheck())
	}
	hc := fthealth.TimedHealthCheck{
		HealthCheck: fthealth.HealthCheck{
			SystemCode:  h.systemCode,
			Name:        h.appName,
			Description: "Checks if all the dependent services are reachable and healthy.",
			Checks:      checks,
		},
		Timeout: 10 * time.Second,
	}
	return fthealth.Handler(hc)
}

func (h healthCheckHandler) GTG() gtg.Status {
	writerCheck := func() gtg.Status {
		return gtgCheck(h.Checker)
	}

	if h.consumer == nil {
		return writerCheck()
	}

	consumerCheck := func() gtg.Status {
		return gtgCheck(h.checkKafkaConnectivity)
	}

	return gtg.FailFastParallelCheck([]gtg.StatusChecker{
		consumerCheck,
		writerCheck,
	})()
}

func (h healthCheckHandler) readQueueCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "read-message-queue-reachable",
		Name:             "Read Message Queue Reachable",
		Severity:         1,
		BusinessImpact:   "Content metadata can't be read from queue. This will negatively impact metadata/annotations availability.",
		TechnicalSummary: "Read message queue is not reachable/healthy",
		PanicGuide:       "https://runbooks.in.ft.com/" + h.systemCode,
		Checker:          h.checkKafkaConnectivity,
	}
}

func (h healthCheckHandler) writerCheck() fthealth.Check {
	return fthealth.Check{
		ID:               "write-message-datastore-reachable",
		Name:             "Write Message Data Store Reachable",
		Severity:         1,
		BusinessImpact:   "Unable to respond to Annotation API requests",
		TechnicalSummary: "Cannot connect to Neo4j a instance with at least one person loaded in it",
		PanicGuide:       "https://runbooks.in.ft.com/" + h.systemCode,
		Checker:          h.Checker,
	}
}

func (h healthCheckHandler) checkKafkaConnectivity() (string, error) {
	if err := h.consumer.ConnectivityCheck(); err != nil {
		return "Error connecting with Kafka", err
	}
	return "Successfully connected to Kafka", nil
}

// Checker does more stuff
//TODO use the shared utility check
func (hc healthCheckHandler) Checker() (string, error) {
	if err := hc.annotationsService.Check(); err != nil {
		return "Error connecting to neo4j", err
	}
	return "Connectivity to neo4j is ok", nil
}

func gtgCheck(handler func() (string, error)) gtg.Status {
	if _, err := handler(); err != nil {
		return gtg.Status{GoodToGo: false, Message: err.Error()}
	}
	return gtg.Status{GoodToGo: true}
}
