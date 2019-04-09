package annotations

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/Financial-Times/go-logger"
	"github.com/Financial-Times/neo-model-utils-go/mapper"
	"github.com/Financial-Times/neo-utils-go/neoutils"
	"github.com/jmcvetta/neoism"
)

var uuidExtractRegex = regexp.MustCompile(".*/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$")

var UnsupportedPredicateErr = errors.New("Unsupported predicate")

// Service interface. Compatible with the baserwftapp service EXCEPT for
// 1) the Write function, which has signature Write(thing interface{}) error...
// 2) the DecodeJson function, which has signature DecodeJSON(*json.Decoder) (thing interface{}, identity string, err error)
// The problem is that we have a list of things, and the uuid is for a related OTHER thing
// TODO - move to implement a shared defined Service interface?
type Service interface {
	Write(contentUUID string, annotationLifecycle string, platformVersion string, tid string, thing interface{}) (err error)
	Read(contentUUID string, tid string, annotationLifecycle string) (thing interface{}, found bool, err error)
	Delete(contentUUID string, tid string, annotationLifecycle string) (found bool, err error)
	Check() (err error)
	DecodeJSON(*json.Decoder) (thing interface{}, err error)
	Count(annotationLifecycle string, platformVersion string) (int, error)
	Initialise() error
}

//holds the Neo4j-specific information
type service struct {
	conn neoutils.NeoConnection
}

const (
	nextVideoAnnotationsLifecycle = "annotations-next-video"
	brightcoveAnnotationLifecycle = "annotations-brightcove"
)

//NewCypherAnnotationsService instantiate driver
func NewCypherAnnotationsService(cypherRunner neoutils.NeoConnection) service {
	return service{cypherRunner}
}

// DecodeJSON decodes to a list of annotations, for ease of use this is a struct itself
func (s service) DecodeJSON(dec *json.Decoder) (interface{}, error) {
	a := Annotations{}
	err := dec.Decode(&a)
	return a, err
}

func (s service) Read(contentUUID string, tid string, annotationLifecycle string) (thing interface{}, found bool, err error) {
	results := []Annotation{}
	//TODO shouldn't return Provenances if none of the scores, agentRole or atTime are set
	statementTemplate := `
			MATCH (c:Thing{uuid:{contentUUID}})-[rel{lifecycle:{annotationLifecycle}}]->(cc:Thing)
			WITH c, cc, rel, {id:cc.uuid,prefLabel:cc.prefLabel,types:labels(cc),predicate:type(rel)} as thing,
			collect(
				{scores:[
					{scoringSystem:'%s', value:rel.relevanceScore},
					{scoringSystem:'%s', value:rel.confidenceScore}],
				agentRole:rel.annotatedBy,
				atTime:rel.annotatedDate}) as provenances
			RETURN thing, provenances ORDER BY thing.id`

	statement := fmt.Sprintf(statementTemplate, relevanceScoringSystem, confidenceScoringSystem)

	query := &neoism.CypherQuery{
		Statement:  statement,
		Parameters: neoism.Props{"contentUUID": contentUUID, "annotationLifecycle": annotationLifecycle},
		Result:     &results,
	}
	logger.WithTransactionID(tid).WithUUID(contentUUID).Debugf("Query returned following results: %v", results)
	if err := s.conn.CypherBatch([]*neoism.CypherQuery{query}); err != nil {
		logger.WithError(err).WithTransactionID(tid).WithUUID(contentUUID).Errorf("Error executing delete queries in neo4j!")
		return Annotations{}, false, err
	}
	logger.WithTransactionID(tid).WithUUID(contentUUID).Debugf("Query returned following results: %v", results)
	if (len(results)) == 0 {
		return Annotations{}, false, nil
	}

	for idx := range results {
		mapToResponseFormat(&results[idx])
	}

	return Annotations(results), true, nil
}

//Delete removes all the annotations for this content. Ignore the nodes on either end -
//may leave nodes that are only 'things' inserted by this writer: clean up
//as a result of this will need to happen externally if required
func (s service) Delete(contentUUID string, tid string, annotationLifecycle string) (bool, error) {

	query := buildDeleteQuery(contentUUID, annotationLifecycle, true)
	logger.WithTransactionID(tid).WithUUID(contentUUID).Debugf("Sending delete queries to neo4j: %v", query)

	if err := s.conn.CypherBatch([]*neoism.CypherQuery{query}); err != nil {
		logger.WithError(err).WithTransactionID(tid).WithUUID(contentUUID).Error("Error executing delete queries in neo4j!")
	}

	stats, err := query.Stats()
	if err != nil {
		logger.WithError(err).WithTransactionID(tid).WithUUID(contentUUID).Error("Error running stats on delete queries")
		return false, err
	}

	return stats.ContainsUpdates, err
}

//Write a set of annotations associated with a piece of content. Any annotations
//already there will be removed
func (s service) Write(contentUUID string, annotationLifecycle string, platformVersion string, tid string, thing interface{}) error {
	annotationsToWrite, ok := thing.(Annotations)
	if ok == false {
		err := fmt.Errorf("thing is not of type Annotations")
		logger.WithTransactionID(tid).WithUUID(contentUUID).Error(err.Error())
		return err
	}
	if contentUUID == "" {
		err := fmt.Errorf("content uuid is required")
		logger.WithTransactionID(tid).WithUUID(contentUUID).Error(err.Error())
		return err
	}

	if err := validateAnnotations(&annotationsToWrite); err != nil {
		logger.WithTransactionID(tid).WithUUID(contentUUID).WithError(err).Error("Validation of supplied annotations failed")
		return err
	}

	if len(annotationsToWrite) == 0 {
		logger.WithTransactionID(tid).WithUUID(contentUUID).Info("No annotations supplied for content")
	}

	queries := append([]*neoism.CypherQuery{}, buildDeleteQuery(contentUUID, annotationLifecycle, false))

	var statements []string
	for _, annotationToWrite := range annotationsToWrite {
		query, err := createAnnotationQuery(contentUUID, annotationToWrite, platformVersion, annotationLifecycle)
		if err != nil {
			logger.WithError(err).WithTransactionID(tid).WithUUID(contentUUID).Error("Create annotation query failed")
			return err
		}
		statements = append(statements, query.Statement)
		queries = append(queries, query)
	}

	logger.WithTransactionID(tid).WithUUID(contentUUID).Debugf("Writing statements to neo4j: %v", statements)
	if err := s.conn.CypherBatch(queries); err != nil {
		logger.WithError(err).WithTransactionID(tid).WithUUID(contentUUID).Error("Error executing write queries in neo4j!")
		return err
	}
	return nil
}

// Check tests neo4j by running a simple cypher query
func (s service) Check() error {
	writableErr := neoutils.CheckWritable(s.conn)
	if writableErr != nil {
		return writableErr
	}

	return neoutils.Check(s.conn)
}

func (s service) Count(annotationLifecycle string, platformVersion string) (int, error) {
	var results []struct {
		Count int `json:"c"`
	}

	query := &neoism.CypherQuery{
		Statement: `MATCH ()-[r{platformVersion:{platformVersion}}]->()
                WHERE r.lifecycle = {lifecycle}
                OR r.lifecycle IS NULL
                RETURN count(r) as c`,
		Parameters: neoism.Props{"platformVersion": platformVersion, "lifecycle": annotationLifecycle},
		Result:     &results,
	}

	err := s.conn.CypherBatch([]*neoism.CypherQuery{query})

	if err != nil {
		logger.WithError(err).Error("Error executing count query in neo4j!")
		return 0, err
	}

	return results[0].Count, nil
}

func (s service) Initialise() error {
	return s.conn.EnsureConstraints(map[string]string{
		"Thing": "uuid",
	})
}

func createAnnotationRelationship(relation string) (statement string) {
	stmt := `
                MERGE (content:Thing{uuid:{contentID}})
                MERGE (concept:Thing{uuid:{conceptID}})
                MERGE (content)-[pred:%s {lifecycle:{annotationLifecycle}}]->(concept)
                SET pred={annProps}
          `
	statement = fmt.Sprintf(stmt, relation)
	return statement
}

func getRelationshipFromPredicate(predicate string) (string, error) {
	if predicate == "" {
		return relations["mentions"], nil
	}

	r, ok := relations[predicate]
	if !ok {
		return "", UnsupportedPredicateErr
	}
	return r, nil
}

func createAnnotationQuery(contentUUID string, ann Annotation, platformVersion string, annotationLifecycle string) (*neoism.CypherQuery, error) {
	query := neoism.CypherQuery{}
	thingID, err := extractUUIDFromURI(ann.Thing.ID)
	if err != nil {
		return nil, err
	}

	//todo temporary change to deal with multiple provenances
	/*if len(ann.Provenances) > 1 {
		return nil, errors.New("Cannot insert a MENTIONS annotation with multiple provenances")
	}*/

	var prov Provenance
	params := map[string]interface{}{}
	params["platformVersion"] = platformVersion
	params["lifecycle"] = annotationLifecycle

	if len(ann.Provenances) >= 1 {
		prov = ann.Provenances[0]
		annotatedBy, annotatedDateEpoch, relevanceScore, confidenceScore, supplied, err := extractDataFromProvenance(&prov)

		if err != nil {
			return nil, err
		}

		if supplied == true {
			if annotatedBy != "" {
				params["annotatedBy"] = annotatedBy
			}
			if prov.AtTime != "" {
				params["annotatedDateEpoch"] = annotatedDateEpoch
				params["annotatedDate"] = prov.AtTime
			}
			params["relevanceScore"] = relevanceScore
			params["confidenceScore"] = confidenceScore
		}
	}

	relation, err := getRelationshipFromPredicate(ann.Thing.Predicate)
	if err != nil {
		return nil, err
	}

	query.Statement = createAnnotationRelationship(relation)
	query.Parameters = map[string]interface{}{
		"contentID":           contentUUID,
		"conceptID":           thingID,
		"annotationLifecycle": annotationLifecycle,
		"annProps":            params,
	}
	return &query, nil
}

func extractDataFromProvenance(prov *Provenance) (string, int64, float64, float64, bool, error) {
	if len(prov.Scores) == 0 {
		return "", -1, -1, -1, false, nil
	}
	var annotatedBy string
	var annotatedDateEpoch int64
	var confidenceScore, relevanceScore float64
	var err error
	if prov.AgentRole != "" {
		annotatedBy, err = extractUUIDFromURI(prov.AgentRole)
	}
	if prov.AtTime != "" {
		annotatedDateEpoch, err = convertAnnotatedDateToEpoch(prov.AtTime)
	}
	relevanceScore, confidenceScore, err = extractScores(prov.Scores)

	if err != nil {
		return "", -1, -1, -1, true, err
	}
	return annotatedBy, annotatedDateEpoch, relevanceScore, confidenceScore, true, nil
}

func extractUUIDFromURI(uri string) (string, error) {
	result := uuidExtractRegex.FindStringSubmatch(uri)
	if len(result) == 2 {
		return result[1], nil
	}
	return "", fmt.Errorf("couldn't extract uuid from uri %s", uri)
}

func convertAnnotatedDateToEpoch(annotatedDateString string) (int64, error) {
	datetimeEpoch, err := time.Parse(time.RFC3339, annotatedDateString)

	if err != nil {
		return 0, err
	}

	return datetimeEpoch.Unix(), nil
}

func extractScores(scores []Score) (float64, float64, error) {
	var relevanceScore, confidenceScore float64
	for _, score := range scores {
		scoringSystem := score.ScoringSystem
		value := score.Value
		switch scoringSystem {
		case relevanceScoringSystem:
			relevanceScore = value
		case confidenceScoringSystem:
			confidenceScore = value
		}
	}
	return relevanceScore, confidenceScore, nil
}

func buildDeleteQuery(contentUUID string, annotationLifecycle string, includeStats bool) *neoism.CypherQuery {
	statement := `	OPTIONAL MATCH (:Thing{uuid:{contentID}})-[r{lifecycle:{annotationLifecycle}}]->(t:Thing)
					DELETE r`

	query := neoism.CypherQuery{
		Statement:    statement,
		Parameters:   neoism.Props{"contentID": contentUUID, "annotationLifecycle": annotationLifecycle, "brightcoveLifecycle": brightcoveAnnotationLifecycle},
		IncludeStats: includeStats}
	return &query
}

func validateAnnotations(annotations *Annotations) error {
	//TODO - for consistency, we should probably just not create the annotation?
	for _, annotation := range *annotations {
		if annotation.Thing.ID == "" {
			return ValidationError{fmt.Sprintf("Concept uuid missing for annotation %+v", annotation)}
		}
	}
	return nil
}

//ValidationError is thrown when the annotations are not valid because mandatory information is missing
type ValidationError struct {
	Msg string
}

func (v ValidationError) Error() string {
	return v.Msg
}

func mapToResponseFormat(ann *Annotation) {
	ann.Thing.ID = mapper.IDURL(ann.Thing.ID)
	// We expect only ONE provenance - provenance value is considered valid even if the AgentRole is not specified. See: v1 - isClassifiedBy
	for idx := range ann.Provenances {
		if ann.Provenances[idx].AgentRole != "" {
			ann.Provenances[idx].AgentRole = mapper.IDURL(ann.Provenances[idx].AgentRole)
		}
	}
}
