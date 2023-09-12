package annotations

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Financial-Times/cm-annotations-ontology/model"

	"github.com/Financial-Times/cm-annotations-ontology/neo4j"

	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"
)

// Service interface. Compatible with the baserwftapp service EXCEPT for
// 1) the Write function, which has signature Write(thing interface{}) error...
// 2) the DecodeJson function, which has signature DecodeJSON(*json.Decoder) (thing interface{}, identity string, err error)
// The problem is that we have a list of things, and the uuid is for a related OTHER thing
// TODO - move to implement a shared defined Service interface?
type Service interface {
	Write(contentUUID string, annotationLifecycle string, platformVersion string, anns interface{}) (bookmark string, err error)
	Read(contentUUID string, bookmark string, annotationLifecycle string) (thing interface{}, found bool, err error)
	Delete(contentUUID string, annotationLifecycle string) (found bool, bookmark string, err error)
	Check() (err error)
	DecodeJSON(*json.Decoder) (thing interface{}, err error)
	Count(annotationLifecycle string, bookmark string, platformVersion string) (int, error)
	Initialise() error
}

// holds the Neo4j-specific information
type service struct {
	driver       *cmneo4j.Driver
	publicAPIURL string
}

// NewCypherAnnotationsService instantiate driver
func NewCypherAnnotationsService(driver *cmneo4j.Driver, publicAPIURL string) (Service, error) {
	_, err := url.ParseRequestURI(publicAPIURL)
	if err != nil {
		return nil, err
	}

	return service{driver: driver, publicAPIURL: publicAPIURL}, nil
}

// DecodeJSON decodes to a list of annotations, for ease of use this is a struct itself
func (s service) DecodeJSON(dec *json.Decoder) (interface{}, error) {
	a := model.Annotations{}
	err := dec.Decode(&a)
	return a, err
}

func (s service) Read(contentUUID string, bookmark string, annotationLifecycle string) (ann interface{}, found bool, err error) {
	query, results := neo4j.GetReadQuery(contentUUID, annotationLifecycle)
	_, err = s.driver.ReadMultiple(query, []string{bookmark})
	if errors.Is(err, cmneo4j.ErrNoResultsFound) {
		return model.Annotations{}, false, nil
	}
	if err != nil {
		return model.Annotations{}, false, fmt.Errorf("error executing delete queries: %w", err)
	}

	mappedResults := *results
	for idx := range mappedResults {
		mapToResponseFormat(&mappedResults[idx], s.publicAPIURL)
	}

	return results, true, nil
}

// Delete removes all the annotations for this content. Ignore the nodes on either end -
// may leave nodes that are only 'things' inserted by this writer: clean up
// as a result of this will need to happen externally if required
func (s service) Delete(contentUUID string, annotationLifecycle string) (bool, string, error) {
	query := neo4j.BuildDeleteQuery(contentUUID, annotationLifecycle, true)

	bookmark, err := s.driver.WriteMultiple([]*cmneo4j.Query{query}, nil)
	if err != nil {
		return false, "", fmt.Errorf("error executing delete queries: %w", err)
	}

	stats, err := query.Summary()
	if err != nil {
		return false, "", fmt.Errorf("error running stats on delete queries: %w", err)
	}

	return stats.Counters().RelationshipsDeleted() > 0, bookmark, err
}

// Write a set of annotations associated with a piece of content. Any annotations
// already there will be removed
func (s service) Write(contentUUID string, annotationLifecycle string, platformVersion string, anns interface{}) (string, error) {
	if contentUUID == "" {
		return "", errors.New("content uuid is required")
	}

	queries := append([]*cmneo4j.Query{}, neo4j.BuildDeleteQuery(contentUUID, annotationLifecycle, false))

	annotations, ok := anns.([]interface{})
	if !ok {
		return "", errors.New("error in casting annotations")
	}

	for _, annotationToWrite := range annotations {
		annotation, ok := annotationToWrite.(map[string]interface{})
		if !ok {
			return "", errors.New("error in casting annotation")
		}

		query, err := neo4j.CreateAnnotationQuery(contentUUID, annotation, platformVersion, annotationLifecycle)
		if err != nil {
			return "", fmt.Errorf("create annotation query failed: %w", err)
		}
		queries = append(queries, query)
	}

	bookmark, err := s.driver.WriteMultiple(queries, nil)
	if err != nil {
		return "", fmt.Errorf("executing write queries in neo4j failed: %w", err)
	}
	return bookmark, nil
}

// Check tests if the service can connect to neo4j by running a simple query
func (s service) Check() error {
	return s.driver.VerifyConnectivity()
}

func (s service) Count(annotationLifecycle string, bookmark string, platformVersion string) (int, error) {
	query, results := neo4j.Count(annotationLifecycle, platformVersion)

	_, err := s.driver.ReadMultiple(query, []string{bookmark})
	if errors.Is(err, cmneo4j.ErrNoResultsFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("executing count query in neo4j failed: %w", err)
	}
	return results[0].Count, nil
}

func (s service) Initialise() error {
	err := s.driver.EnsureConstraints(map[string]string{
		"Thing": "uuid",
	})

	return err
}

func mapToResponseFormat(ann *model.Annotation, publicAPIURL string) {
	ann.ID = thingURL(ann.ID, publicAPIURL)
}

func thingURL(uuid, baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/things/" + uuid
}
