package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	logger "github.com/Financial-Times/go-logger/v2"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

const (
	lifecyclePropertyName = "annotationLifecycle"
)

//service def
type httpHandler struct {
	annotationsService annotations.Service
	forwarder          forwarder.QueueForwarder
	originMap          map[string]string
	lifecycleMap       map[string]string
	messageType        string
	log                *logger.UPPLogger
}

// GetAnnotations returns a view of the annotations written - it is NOT the public annotations API, and
// the response format should be consistent with the PUT request body format
func (hh *httpHandler) GetAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		writeJSONError(w, "uuid required", http.StatusBadRequest)
		return
	}

	lifecycle := vars[lifecyclePropertyName]
	if lifecycle == "" {
		writeJSONError(w, "annotationLifecycle required", http.StatusBadRequest)
		return
	} else if _, ok := hh.lifecycleMap[lifecycle]; !ok {
		writeJSONError(w, "annotationLifecycle not supported by this application", http.StatusBadRequest)
		return
	}

	tid := transactionidutils.GetTransactionIDFromRequest(r)
	annotations, found, err := hh.annotationsService.Read(uuid, tid, lifecycle)
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed getting annotations")
		msg := fmt.Sprintf("Error getting annotations (%v)", err)
		writeJSONError(w, msg, http.StatusServiceUnavailable)
		return
	}
	if !found {
		writeJSONError(w, fmt.Sprintf("No annotations found for content with uuid %s.", uuid), http.StatusNotFound)
		return
	}
	annotationJson, _ := json.Marshal(annotations)
	hh.log.Debugf("Annotations for content (uuid:%s): %s\n", uuid, annotationJson)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(annotations)
}

// DeleteAnnotations will delete all the annotations for a piece of content
func (hh *httpHandler) DeleteAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")

	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		writeJSONError(w, "uuid required", http.StatusBadRequest)
		return
	}

	lifecycle := vars[lifecyclePropertyName]
	if lifecycle == "" {
		writeJSONError(w, "annotationLifecycle required", http.StatusBadRequest)
		return
	} else if _, ok := hh.lifecycleMap[lifecycle]; !ok {
		writeJSONError(w, "annotationLifecycle not supported by this application", http.StatusBadRequest)
		return
	}

	tid := transactionidutils.GetTransactionIDFromRequest(r)
	found, err := hh.annotationsService.Delete(uuid, tid, lifecycle)
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed deleting annotations")
		writeJSONError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if !found {
		writeJSONError(w, fmt.Sprintf("No annotations found for content with uuid %s.", uuid), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusNoContent)
	w.Write([]byte(jsonMessage(fmt.Sprintf("Annotations for content %s deleted", uuid))))
}

func (hh *httpHandler) CountAnnotations(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	lifecycle := vars[lifecyclePropertyName]
	if lifecycle == "" {
		writeJSONError(w, "annotationLifecycle required", http.StatusBadRequest)
		return
	} else if _, ok := hh.lifecycleMap[lifecycle]; !ok {
		writeJSONError(w, "annotationLifecycle not supported by this application", http.StatusBadRequest)
		return
	}

	platformVersion, found := hh.lifecycleMap[lifecycle]
	if !found {
		writeJSONError(w, "platformVersion not found for this annotation lifecycle", http.StatusBadRequest)
		return
	}

	count, err := hh.annotationsService.Count(lifecycle, platformVersion)

	w.Header().Add("Content-Type", "application/json")

	if err != nil {
		writeJSONError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	enc := json.NewEncoder(w)

	if err := enc.Encode(count); err != nil {
		writeJSONError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
}

// PutAnnotations handles the replacement of a set of annotations for a given bit of content
func (hh *httpHandler) PutAnnotations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	if err := isContentTypeJSON(r); err != nil {
		http.Error(w, string(jsonMessage(err.Error())), http.StatusBadRequest)
		return
	}
	vars := mux.Vars(r)
	uuid := vars["uuid"]
	if uuid == "" {
		writeJSONError(w, "uuid required", http.StatusBadRequest)
		return
	}

	lifecycle := vars[lifecyclePropertyName]
	if lifecycle == "" {
		writeJSONError(w, "annotationLifecycle required for uuid %s"+uuid, http.StatusBadRequest)
		return
	}

	platformVersion, ok := hh.lifecycleMap[lifecycle]
	if !ok {
		writeJSONError(w, "annotationLifecycle not supported by this application", http.StatusBadRequest)
		return
	}

	var originSystem string
	for k, v := range hh.originMap {
		if v == lifecycle {
			originSystem = k
			break
		}
	}
	if originSystem == "" {
		writeJSONError(w, "No Origin-System-Id could be deduced from the lifecycle parameter", http.StatusBadRequest)
		return
	}

	anns, err := decode(r.Body)
	if err != nil {
		msg := fmt.Sprintf("Error (%v) parsing annotation request", err)
		writeJSONError(w, msg, http.StatusBadRequest)
		return
	}

	tid := transactionidutils.GetTransactionIDFromRequest(r)
	err = hh.annotationsService.Write(uuid, lifecycle, platformVersion, tid, anns)
	if err == annotations.UnsupportedPredicateErr {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("invalid predicate provided")
		writeJSONError(w, "Please provide a valid predicate", http.StatusBadRequest)
		return
	}

	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed writing annotations")
		msg := fmt.Sprintf("Error creating annotations (%v)", err)
		if _, ok := err.(annotations.ValidationError); ok {
			writeJSONError(w, msg, http.StatusBadRequest)
			return
		}
		hh.log.WithMonitoringEvent("SaveNeo4j", tid, hh.messageType).WithUUID(uuid).WithError(err).Error(msg)
		writeJSONError(w, msg, http.StatusServiceUnavailable)
		return
	}
	hh.log.WithMonitoringEvent("SaveNeo4j", tid, hh.messageType).WithUUID(uuid).Infof("%s successfully written in Neo4j", hh.messageType)

	if hh.forwarder != nil {
		hh.log.WithTransactionID(tid).WithUUID(uuid).Debug("Forwarding message to the next queue")
		err = hh.forwarder.SendMessage(tid, originSystem, platformVersion, uuid, anns)
		if err != nil {
			msg := "Failed to forward message to queue"
			hh.log.WithTransactionID(tid).WithUUID(uuid).WithError(err).Error(msg)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(jsonMessage(msg)))
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(jsonMessage(fmt.Sprintf("Annotations for content %s created", uuid))))
	return
}

func writeJSONError(w http.ResponseWriter, errorMsg string, statusCode int) {
	w.WriteHeader(statusCode)
	fmt.Fprintln(w, fmt.Sprintf("{\"message\": \"%s\"}", errorMsg))
}

func jsonMessage(msgText string) []byte {
	return []byte(fmt.Sprintf(`{"message":"%s"}`, msgText))
}

func decode(body io.Reader) (annotations.Annotations, error) {
	var anns annotations.Annotations
	err := json.NewDecoder(body).Decode(&anns)
	return anns, err
}

func isContentTypeJSON(r *http.Request) error {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") {
		return errors.New("Http Header 'Content-Type' is not 'application/json', this is a JSON API")
	}
	return nil
}
