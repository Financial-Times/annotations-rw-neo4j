package annotations

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"
)

var uuidExtractRegex = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")

var UnsupportedPredicateErr = errors.New("Unsupported predicate")

// Service interface. Compatible with the baserwftapp service EXCEPT for
// 1) the Write function, which has signature Write(thing interface{}) error...
// 2) the DecodeJson function, which has signature DecodeJSON(*json.Decoder) (thing interface{}, identity string, err error)
// The problem is that we have a list of things, and the uuid is for a related OTHER thing
// TODO - move to implement a shared defined Service interface?
type Service interface {
	Write(contentUUID string, annotationLifecycle string, platformVersion string, thing interface{}) (bookmark string, err error)
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

const (
	nextVideoAnnotationsLifecycle = "annotations-next-video"
)

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
	a := Annotations{}
	err := dec.Decode(&a)
	return a, err
}

func (s service) Read(contentUUID string, bookmark string, annotationLifecycle string) (thing interface{}, found bool, err error) {
	results := []Annotation{}
	statement := `
			MATCH (c:Thing{uuid:$contentUUID})-[rel{lifecycle:$annotationLifecycle}]->(cc:Thing)
			RETURN 
				cc.uuid as id,
				cc.preflabel as prefLabel,
				labels(cc) as types,
				type(rel) as predicate,
				rel.relevanceScore as relevanceScore,
				rel.confidenceScore as confidenceScore,
				rel.annotatedBy as annotatedBy,
				rel.annotatedDate as annotatedDate
			ORDER BY id`

	query := []*cmneo4j.Query{{
		Cypher: statement,
		Params: map[string]interface{}{
			"contentUUID":         contentUUID,
			"annotationLifecycle": annotationLifecycle,
		},
		Result: &results,
	}}
	_, err = s.driver.ReadMultiple(query, []string{bookmark})
	if errors.Is(err, cmneo4j.ErrNoResultsFound) {
		return Annotations{}, false, nil
	}
	if err != nil {
		return Annotations{}, false, fmt.Errorf("error executing delete queries: %w", err)
	}

	for idx := range results {
		mapToResponseFormat(&results[idx], s.publicAPIURL)
	}

	return Annotations(results), true, nil
}

// Delete removes all the annotations for this content. Ignore the nodes on either end -
// may leave nodes that are only 'things' inserted by this writer: clean up
// as a result of this will need to happen externally if required
func (s service) Delete(contentUUID string, annotationLifecycle string) (bool, string, error) {
	query := buildDeleteQuery(contentUUID, annotationLifecycle, true)

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
func (s service) Write(contentUUID string, annotationLifecycle string, platformVersion string, thing interface{}) (string, error) {
	annotationsToWrite, ok := thing.(Annotations)
	if ok == false {
		return "", errors.New("thing is not of type Annotations")
	}
	if contentUUID == "" {
		return "", errors.New("content uuid is required")
	}

	if err := validateAnnotations(&annotationsToWrite); err != nil {
		return "", err
	}

	queries := append([]*cmneo4j.Query{}, buildDeleteQuery(contentUUID, annotationLifecycle, false))

	for _, annotationToWrite := range annotationsToWrite {
		query, err := createAnnotationQuery(contentUUID, annotationToWrite, platformVersion, annotationLifecycle)
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
	var results []struct {
		Count int `json:"c"`
	}

	query := []*cmneo4j.Query{{
		Cypher: `MATCH ()-[r{platformVersion:$platformVersion}]->()
                WHERE r.lifecycle = $lifecycle
                OR r.lifecycle IS NULL
                RETURN count(r) as c`,
		Params: map[string]interface{}{
			"platformVersion": platformVersion,
			"lifecycle":       annotationLifecycle,
		},
		Result: &results,
	}}

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

func createAnnotationRelationship(relation string) (statement string) {
	stmt := `
                MERGE (content:Thing{uuid:$contentID})
                MERGE (concept:Thing{uuid:$conceptID})
                MERGE (content)-[pred:%s {lifecycle:$annotationLifecycle}]->(concept)
                SET pred=$annProps
          `
	statement = fmt.Sprintf(stmt, relation)
	return statement
}

func getRelationshipFromPredicate(predicate string) (string, error) {
	r, ok := relations[extractPredicateFromURI(predicate)]
	if !ok {
		return "", UnsupportedPredicateErr
	}
	return r, nil
}

func createAnnotationQuery(contentUUID string, ann Annotation, platformVersion string, annotationLifecycle string) (*cmneo4j.Query, error) {
	thingID, err := extractUUIDFromURI(ann.ID)
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{}
	params["platformVersion"] = platformVersion
	params["lifecycle"] = annotationLifecycle

	if ann.AnnotatedBy != "" {
		params["annotatedBy"], err = extractUUIDFromURI(ann.AnnotatedBy)
		if err != nil {
			return nil, err
		}
	}
	if ann.AnnotatedDate != "" {
		params["annotatedDateEpoch"] = ann.AnnotatedDateEpoch
		params["annotatedDate"] = ann.AnnotatedDate
	}
	if ann.RelevanceScore != 0.0 {
		params["relevanceScore"] = ann.RelevanceScore
	}
	if ann.ConfidenceScore != 0.0 {
		params["confidenceScore"] = ann.ConfidenceScore
	}

	relation, err := getRelationshipFromPredicate(ann.Predicate)
	if err != nil {
		return nil, err
	}

	query := &cmneo4j.Query{
		Cypher: createAnnotationRelationship(relation),
		Params: map[string]interface{}{
			"contentID":           contentUUID,
			"conceptID":           thingID,
			"annotationLifecycle": annotationLifecycle,
			"annProps":            params,
		},
	}

	return query, nil
}

func buildDeleteQuery(contentUUID string, annotationLifecycle string, includeStats bool) *cmneo4j.Query {
	statement := `OPTIONAL MATCH (:Thing{uuid:$contentID})-[r{lifecycle:$annotationLifecycle}]->(t:Thing)
				  DELETE r`
	query := &cmneo4j.Query{
		Cypher: statement,
		Params: map[string]interface{}{
			"contentID":           contentUUID,
			"annotationLifecycle": annotationLifecycle,
		},
		IncludeSummary: includeStats,
	}
	return query
}

func validateAnnotations(annotations *Annotations) error {
	//TODO - for consistency, we should probably just not create the annotation?
	for _, annotation := range *annotations {
		if annotation.ID == "" {
			return ValidationError{fmt.Sprintf("Concept uuid missing for annotation %+v", annotation)}
		}
	}
	return nil
}

// ValidationError is thrown when the annotations are not valid because mandatory information is missing
type ValidationError struct {
	Msg string
}

func (v ValidationError) Error() string {
	return v.Msg
}

func mapToResponseFormat(ann *Annotation, publicAPIURL string) {
	ann.ID = thingURL(ann.ID, publicAPIURL)
	if ann.AnnotatedBy != "" {
		ann.AnnotatedBy = thingURL(ann.AnnotatedBy, publicAPIURL)
	}
}

func thingURL(uuid, baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/things/" + uuid
}

func extractUUIDFromURI(uri string) (string, error) {
	result := uuidExtractRegex.FindStringSubmatch(uri)
	if len(result) == 2 {
		return result[1], nil
	}
	return "", fmt.Errorf("couldn't extract uuid from uri %s", uri)
}

func extractPredicateFromURI(uri string) string {
	_, result := path.Split(uri)
	return result
}
