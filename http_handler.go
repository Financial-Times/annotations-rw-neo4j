package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/annotations"
	"github.com/Financial-Times/annotations-rw-neo4j/v4/forwarder"

	logger "github.com/Financial-Times/go-logger/v2"
	transactionidutils "github.com/Financial-Times/transactionid-utils-go"

	"github.com/gorilla/mux"
)

const (
	lifecyclePropertyName = "annotationLifecycle"
	bookmarkHeader        = "Neo4j-Bookmark"
	publicationHeader     = "Publication"
)

// service def
type httpHandler struct {
	validator          jsonValidator
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
	bookmark := r.Header.Get(bookmarkHeader)
	annotations, found, err := hh.annotationsService.Read(uuid, bookmark, lifecycle)
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
	err = json.NewEncoder(w).Encode(annotations)
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("writing response")
	}
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
	found, bookmark, err := hh.annotationsService.Delete(uuid, lifecycle)
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed deleting annotations")
		writeJSONError(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if !found {
		writeJSONError(w, fmt.Sprintf("No annotations found for content with uuid %s.", uuid), http.StatusNotFound)
		return
	}
	w.Header().Add(bookmarkHeader, bookmark)
	w.WriteHeader(http.StatusNoContent)
	_, err = w.Write(jsonMessage(fmt.Sprintf("Annotations for content %s deleted", uuid)))
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("writing response")
	}
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

	bookmark := r.Header.Get(bookmarkHeader)
	count, err := hh.annotationsService.Count(lifecycle, bookmark, platformVersion)

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
	for _, ann := range anns {
		err = hh.validator.Validate(ann)
		if err != nil {
			hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed validating annotations")
			msg := fmt.Sprintf("Error validating annotations (%v)", err)
			writeJSONError(w, msg, http.StatusBadRequest)
			return
		}
	}

	var publication []string
	pubStr := r.Header.Get(publicationHeader)
	if pubStr != "" {
		publication = strings.Split(r.Header.Get(publicationHeader), ",")
	}
	bookmark, err := hh.annotationsService.Write(uuid, lifecycle, platformVersion, toSliceOfInterface(publication), anns)
	if err != nil {
		hh.log.WithUUID(uuid).WithTransactionID(tid).WithError(err).Error("failed writing annotations")
		msg := fmt.Sprintf("Error creating annotations (%v)", err)
		hh.log.WithMonitoringEvent("SaveNeo4j", tid, hh.messageType).WithUUID(uuid).WithError(err).Error(msg)
		writeJSONError(w, msg, http.StatusServiceUnavailable)
		return
	}
	hh.log.WithMonitoringEvent("SaveNeo4j", tid, hh.messageType).WithUUID(uuid).Infof("%s successfully written in Neo4j", hh.messageType)

	if hh.forwarder != nil {
		hh.log.WithTransactionID(tid).WithUUID(uuid).Debug("Forwarding message to the next queue")
		err = hh.forwarder.SendMessage(tid, originSystem, bookmark, platformVersion, uuid, anns, publication)
		if err != nil {
			msg := "Failed to forward message to queue"
			hh.log.WithTransactionID(tid).WithUUID(uuid).WithError(err).Error(msg)
			w.WriteHeader(http.StatusInternalServerError)
			_, err = w.Write(jsonMessage(msg))
			if err != nil {
				hh.log.WithTransactionID(tid).WithUUID(uuid).WithError(err).Error("writing response")
			}
			return
		}
	}

	w.Header().Add(bookmarkHeader, bookmark)
	w.WriteHeader(http.StatusCreated)
	_, err = w.Write(jsonMessage(fmt.Sprintf("Annotations for content %s created", uuid)))
	if err != nil {
		hh.log.WithTransactionID(tid).WithUUID(uuid).WithError(err).Error("writing response")
	}
}

func writeJSONError(w http.ResponseWriter, errorMsg string, statusCode int) {
	w.WriteHeader(statusCode)
	fmt.Println(w, fmt.Sprintf("{\"message\": \"%s\"}", errorMsg))
}

func jsonMessage(msgText string) []byte {
	return []byte(fmt.Sprintf(`{"message":"%s"}`, msgText))
}

func decode(body io.Reader) ([]interface{}, error) {
	var anns []interface{}
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

func toSliceOfInterface(s []string) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = v
	}
	return result
}
