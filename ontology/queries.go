package ontology

import (
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"
)

var uuidExtractRegex = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")

var conceptProperties = strings.Split(os.Getenv("CONCEPT_PROPERTIES"), ";")

func GetReadQuery(contentUUID string, annotationLifecycle string) ([]*cmneo4j.Query, *[]Annotation) {
	results := []Annotation{}
	return []*cmneo4j.Query{{
		Cypher: `
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
			ORDER BY id`,
		Params: map[string]interface{}{
			"contentUUID":         contentUUID,
			"annotationLifecycle": annotationLifecycle,
		},
		Result: &results,
	}}, &results
}

func BuildDeleteQuery(contentUUID string, annotationLifecycle string, includeStats bool) *cmneo4j.Query {
	return &cmneo4j.Query{
		Cypher: `OPTIONAL MATCH (:Thing{uuid:$contentID})-[r{lifecycle:$annotationLifecycle}]->(t:Thing)
				  DELETE r`,
		Params: map[string]interface{}{
			"contentID":           contentUUID,
			"annotationLifecycle": annotationLifecycle,
		},
		IncludeSummary: includeStats,
	}
}

func Count(annotationLifecycle string, platformVersion string) ([]*cmneo4j.Query, []struct {
	Count int `json:"c"`
}) {
	var results []struct {
		Count int `json:"c"`
	}

	return []*cmneo4j.Query{{
		Cypher: `MATCH ()-[r{platformVersion:$platformVersion}]->()
                WHERE r.lifecycle = $lifecycle
                OR r.lifecycle IS NULL
                RETURN count(r) as c`,
		Params: map[string]interface{}{
			"platformVersion": platformVersion,
			"lifecycle":       annotationLifecycle,
		},
		Result: &results,
	}}, results
}

func CreateAnnotationQuery(contentUUID string, ann map[string]interface{}, platformVersion string, annotationLifecycle string) (*cmneo4j.Query, error) {
	thingID, err := extractUUIDFromURI(fmt.Sprint(ann["id"]))
	if err != nil {
		return nil, err
	}

	ann["platformVersion"] = platformVersion
	ann["lifecycle"] = annotationLifecycle

	relation := getRelationshipFromPredicate(fmt.Sprint(ann["predicate"]))
	// Remove predicate from the cypher query parameters
	delete(ann, "predicate")

	if len(conceptProperties) == 0 {
		return nil, errors.New("CONCEPT_PROPERTIES environment variable is not set")
	}
	// Remove concept properties from the cypher query parameters
	for _, conceptProperty := range conceptProperties {
		delete(ann, conceptProperty)
	}

	query := &cmneo4j.Query{
		Cypher: createAnnotationRelationship(relation),
		Params: map[string]interface{}{
			"contentID":           contentUUID,
			"conceptID":           thingID,
			"annotationLifecycle": annotationLifecycle,
			"annProps":            ann,
		},
	}

	return query, nil
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

func getRelationshipFromPredicate(predicate string) string {
	r, ok := Relations[extractPredicateFromURI(predicate)]
	if !ok {
		return ""
	}
	return r
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
