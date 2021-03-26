// +build integration

package annotations

import (
	"fmt"
	"os"
	"testing"

	"github.com/Financial-Times/go-logger"
	"github.com/Financial-Times/neo-utils-go/neoutils"
	"github.com/jmcvetta/neoism"
	"github.com/stretchr/testify/assert"
)

var annotationsService Service

const (
	brandUUID                = "8e21cbd4-e94b-497a-a43b-5b2309badeb3"
	v1PlatformVersion        = "v1"
	nextVideoPlatformVersion = "next-video"
	contentLifecycle         = "content"
	v1AnnotationLifecycle    = "annotations-v1"
	tid                      = "transaction_id"
)

func TestConstraintsApplied(t *testing.T) {
	assert := assert.New(t)
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	defer cleanDB(t, assert)

	err := annotationsService.Initialise()
	assert.NoError(err)

	testSetupQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}}) SET n :Thing`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	err = conn.CypherBatch([]*neoism.CypherQuery{testSetupQuery})
	assert.NoError(err, "Error setting up Test data")
	testQuery := &neoism.CypherQuery{
		Statement: `CREATE (n:Thing {uuid:{contentUuid}}) SET n :Thing`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}
	expectErr := conn.CypherBatch([]*neoism.CypherQuery{testQuery})
	assert.Error(expectErr, "DB constraint is not applied correctly")
}

func TestWriteFailsWhenNoConceptIDSupplied(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)

	conceptWithoutID := Annotations{Annotation{
		Thing: Thing{
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			}},
		Provenances: []Provenance{
			{
				Scores: []Score{
					{ScoringSystem: "http://api.ft.com/scoringsystem/FT-RELEVANCE-SYSTEM", Value: 0.9},
					{ScoringSystem: "http://api.ft.com/scoringsystem/FT-CONFIDENCE-SYSTEM", Value: 0.8},
				},
				AgentRole: "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
				AtTime:    "2016-01-01T19:43:47.314Z",
			},
		},
	}}

	err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, conceptWithoutID)
	assert.Error(err, "Should have failed to write annotation")
	_, ok := err.(ValidationError)
	assert.True(ok, "Should have returned a validation error")
}

func TestWriteFailsForInvalidPredicate(t *testing.T) {
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	conceptWithInvalidPredicate := Annotation{
		Thing: Thing{ID: fmt.Sprintf("http://api.ft.com/things/%s", oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/person/Person",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "hasAFakePredicate",
		},
		Provenances: []Provenance{
			{
				Scores: []Score{
					{ScoringSystem: "http://api.ft.com/scoringsystem/FT-RELEVANCE-SYSTEM", Value: 0.9},
					{ScoringSystem: "http://api.ft.com/scoringsystem/FT-CONFIDENCE-SYSTEM", Value: 0.8},
				},
				AgentRole: "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
				AtTime:    "2016-01-01T19:43:47.314Z",
			},
		},
	}

	err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, Annotations{conceptWithInvalidPredicate})
	assert.EqualError(t, err, "create annotation query failed: Unsupported predicate")
}

func TestDeleteRemovesAnnotationsButNotConceptsOrContent(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	annotationsToDelete := exampleConcepts(conceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToDelete), "Failed to write annotation")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, annotationsToDelete)

	deleted, err := annotationsService.Delete(contentUUID, tid, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s: %s", contentUUID, err)
	assert.NoError(err, "Error deleting annotation for content uuid %, conceptUUID %s", contentUUID, conceptUUID)

	anns, found, err := annotationsService.Read(contentUUID, tid, v2AnnotationLifecycle)

	assert.Equal(Annotations{}, anns, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.False(found, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.NoError(err, "Error trying to find annotation for content %s", contentUUID)

	checkNodeIsStillPresent(contentUUID, t)
	checkNodeIsStillPresent(conceptUUID, t)

	err = deleteNode(conn, contentUUID)
	assert.NoError(err, "Error trying to delete content node with uuid %s, err=%v", contentUUID, err)
	err = deleteNode(conn, conceptUUID)
	assert.NoError(err, "Error trying to delete concept node with uuid %s, err=%v", conceptUUID, err)
}

func TestWriteAllValuesPresent(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, annotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID})
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithoutLifecycle(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	defer cleanDB(t, assert)

	testSetupQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}}) SET n :Thing
		MERGE (b:Brand{uuid:{brandUuid}}) SET b :Concept:Thing
		CREATE (n)-[rel:IS_CLASSIFIED_BY{platformVersion:{platformVersion}}]->(b)`,
		Parameters: map[string]interface{}{
			"contentUuid":     contentUUID,
			"brandUuid":       brandUUID,
			"platformVersion": v2PlatformVersion,
		},
	}

	err := conn.CypherBatch([]*neoism.CypherQuery{testSetupQuery})
	assert.NoError(err)

	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")
	checkRelationship(t, assert, contentUUID, "v2")

	deleted, err := annotationsService.Delete(contentUUID, tid, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[:IS_CLASSIFIED_BY]->(b:Brand) RETURN b.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"brandUuid":   brandUUID,
		},
		Result: &result,
	}

	readErr := conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithContentLifeCycle(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	defer cleanDB(t, assert)
	contentQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}}) SET n :Thing
		MERGE (b:Brand{uuid:{brandUuid}}) SET b :Concept:Thing
		CREATE (n)-[rel:IS_CLASSIFIED_BY{platformVersion:{platformVersion}, lifecycle: {lifecycle}}]->(b)`,
		Parameters: map[string]interface{}{
			"contentUuid":     contentUUID,
			"brandUuid":       brandUUID,
			"platformVersion": v2PlatformVersion,
			"lifecycle":       contentLifecycle,
		},
	}

	err := conn.CypherBatch([]*neoism.CypherQuery{contentQuery})
	assert.NoError(err, "Error c for content uuid %s", contentUUID)

	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")
	checkRelationship(t, assert, contentUUID, "v2")

	deleted, err := annotationsService.Delete(contentUUID, tid, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[:IS_CLASSIFIED_BY]->(b:Brand) RETURN b.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"brandUuid":   brandUUID,
		},
		Result: &result,
	}

	readErr := conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesRemoveExistingIsClassifiedForV1TermsAndTheirRelationships(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")

	defer cleanDB(t, assert)
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)

	createContentQuery := &neoism.CypherQuery{
		Statement: `MERGE (c:Content{uuid:{contentUuid}}) SET c :Thing RETURN c.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	assert.NoError(conn.CypherBatch([]*neoism.CypherQuery{createContentQuery}))

	contentQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}})
		 	    MERGE (a:Thing{uuid:{conceptUUID}})
			    CREATE (n)-[rel1:MENTIONS{lifecycle:"annotations-v2"}]->(a)
			    MERGE (b:Thing{uuid:{secondConceptUUID}})
			    CREATE (n)-[rel2:IS_CLASSIFIED_BY{lifecycle:"annotations-v1"}]->(b)`,
		Parameters: map[string]interface{}{
			"contentUuid":       contentUUID,
			"conceptUUID":       conceptUUID,
			"secondConceptUUID": secondConceptUUID,
		},
	}

	assert.NoError(conn.CypherBatch([]*neoism.CypherQuery{contentQuery}))

	assert.NoError(annotationsService.Write(contentUUID, v1AnnotationLifecycle, v1PlatformVersion, tid, exampleConcepts(conceptUUID)), "Failed to write annotation")
	found, err := annotationsService.Delete(contentUUID, tid, v1AnnotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	//CHECK THAT ALL THE v1 annotations were updated
	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[r]->(b:Thing) where r.lifecycle={lifecycle} RETURN b.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"lifecycle":   v1AnnotationLifecycle,
		},
		Result: &result,
	}

	readErr := conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.Empty(result)

	//CHECK THAT V2 annotations were not deleted
	getContentQuery = &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[r]->(b:Thing) where r.lifecycle={lifecycle} RETURN b.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"lifecycle":   v2AnnotationLifecycle,
		},
		Result: &result,
	}

	readErr = conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.NotEmpty(result)

	//Delete v2 annotations
	removeRelationshipQuery := &neoism.CypherQuery{
		Statement: `
			MATCH (b:Thing {uuid:{conceptUUID}})<-[rel]-(t:Thing)
			where rel.platformVersion = "v2"
			DELETE rel
		`,
		Parameters: map[string]interface{}{
			"conceptUUID": conceptUUID,
		},
	}

	assert.NoError(conn.CypherBatch([]*neoism.CypherQuery{removeRelationshipQuery}))
}

func TestWriteAndReadMultipleAnnotations(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)

	multiConceptAnnotations := Annotations{
		Annotation{
			Thing: Thing{ID: getURI(conceptUUID),
				PrefLabel: "prefLabel",
				Types: []string{
					"http://www.ft.com/ontology/product/Brand",
					"http://www.ft.com/ontology/core/Thing",
					"http://www.ft.com/ontology/concept/Concept",
				},
				Predicate: "hasBrand",
			},
			Provenances: []Provenance{
				{
					Scores: []Score{
						{ScoringSystem: relevanceScoringSystem, Value: 0.9},
						{ScoringSystem: confidenceScoringSystem, Value: 0.8},
					},
					AgentRole: "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
					AtTime:    "2016-01-01T19:43:47.314Z",
				},
			},
		},
		Annotation{
			Thing: Thing{ID: getURI(secondConceptUUID),
				PrefLabel: "prefLabel",
				Types: []string{
					"http://www.ft.com/ontology/organisation/Organisation",
					"http://www.ft.com/ontology/core/Thing",
					"http://www.ft.com/ontology/concept/Concept",
				},
			},
			Provenances: []Provenance{
				{
					Scores: []Score{
						{ScoringSystem: relevanceScoringSystem, Value: 0.4},
						{ScoringSystem: confidenceScoringSystem, Value: 0.5},
					},
					AgentRole: "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
					AtTime:    "2016-01-01T19:43:47.314Z",
				},
			},
		},
	}

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, multiConceptAnnotations), "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, multiConceptAnnotations)
	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, secondConceptUUID})
}

func TestIfProvenanceGetsWrittenWithEmptyAgentRoleAndTimeValues(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, conceptWithoutAgent), "Failed to write annotation")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, conceptWithoutAgent)
	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID})
}

func TestNextVideoAnnotationsUpdatesAnnotations(t *testing.T) {
	assert := assert.New(t)
	defer cleanDB(t, assert)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)

	contentQuery := &neoism.CypherQuery{
		Statement: `CREATE (n:Thing {uuid:{contentUuid}})
		 	    CREATE (a:Thing{uuid:{conceptUuid}})
			    CREATE (n)-[rel:MENTIONS{platformVersion:{platformVersion}, lifecycle:{lifecycle}}]->(a)`,
		Parameters: map[string]interface{}{
			"contentUuid":     contentUUID,
			"conceptUuid":     conceptUUID,
			"platformVersion": nextVideoAnnotationsLifecycle,
			"lifecycle":       nextVideoAnnotationsLifecycle,
		},
	}

	err := conn.CypherBatch([]*neoism.CypherQuery{contentQuery})
	assert.NoError(err, "Error creating test data in database.")

	assert.NoError(annotationsService.Write(contentUUID, nextVideoAnnotationsLifecycle, nextVideoPlatformVersion, tid, exampleConcepts(secondConceptUUID)), "Failed to write annotation.")

	result := []struct {
		Lifecycle       string `json:"r.lifecycle"`
		PlatformVersion string `json:"r.platformVersion"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[r]->(b:Thing {uuid:{conceptUuid}}) RETURN r.lifecycle, r.platformVersion`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"conceptUuid": secondConceptUUID,
		},
		Result: &result,
	}

	readErr := conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})

	assert.NoError(readErr)
	assert.Equal(1, len(result), "Relationships size worng.")

	if len(result) > 0 {
		assert.Equal(nextVideoPlatformVersion, result[0].PlatformVersion, "Platform version wrong.")
		assert.Equal(nextVideoAnnotationsLifecycle, result[0].Lifecycle, "Lifecycle wrong.")
	}
}

func TestUpdateWillRemovePreviousAnnotations(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	oldAnnotationsToWrite := exampleConcepts(oldConceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, oldAnnotationsToWrite), "Failed to write annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, oldAnnotationsToWrite)

	updatedAnnotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, updatedAnnotationsToWrite), "Failed to write updated annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, updatedAnnotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, oldConceptUUID})
}

func getNeoConnection(t *testing.T) neoutils.NeoConnection {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")

	url := os.Getenv("NEO4J_TEST_URL")
	if url == "" {
		url = "http://localhost:7474/db/data"
	}

	conf := neoutils.DefaultConnectionConfig()
	conf.Transactional = false
	db, err := neoutils.Connect(url, conf)
	assert.NoError(err, "Failed to connect to Neo4j")
	return db
}

func readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t *testing.T, contentUUID string, annotationLifecycle string, expectedAnnotations []Annotation) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	storedThings, found, err := annotationsService.Read(contentUUID, tid, annotationLifecycle)
	storedAnnotations := storedThings.(Annotations)

	assert.NoError(err, "Error finding annotations for contentUUID %s", contentUUID)
	assert.True(found, "Didn't find annotations for contentUUID %s", contentUUID)
	assert.Equal(len(expectedAnnotations), len(storedAnnotations), "Didn't get the same number of annotations")
	for idx, expectedAnnotation := range expectedAnnotations {
		storedAnnotation := storedAnnotations[idx]
		assert.EqualValues(expectedAnnotation.Provenances, storedAnnotation.Provenances, "Provenances not the same")

		// In annotations write, we don't store anything other than ID for the concept (so type will only be 'Thing' and pref label will not
		// be present UNLESS the concept has been written by some other system)
		assert.Equal(expectedAnnotation.Thing.ID, storedAnnotation.Thing.ID, "Thing ID not the same")

		expectedPredicate, err := getRelationshipFromPredicate(expectedAnnotation.Thing.Predicate)
		assert.NoError(err, "error getting relationship from predicate %s", expectedAnnotation.Thing.Predicate)
		assert.Equal(expectedPredicate, storedAnnotation.Thing.Predicate, "Thing Predicates not the same")
	}
}

func checkNodeIsStillPresent(uuid string, t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	results := []struct {
		UUID string `json:"uuid"`
	}{}

	query := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{uuid}}) return n.uuid
		as uuid`,
		Parameters: map[string]interface{}{
			"uuid": uuid,
		},
		Result: &results,
	}

	err := conn.CypherBatch([]*neoism.CypherQuery{query})
	assert.NoError(err, "UnexpectedError")
	assert.True(len(results) == 1, "Didn't find a node")
	assert.Equal(uuid, results[0].UUID, "Did not find correct node")
}

func checkRelationship(t *testing.T, assert *assert.Assertions, contentID string, platformVersion string) {
	countQuery := `Match (t:Thing {uuid: {contentID}})-[r {lifecycle: {lifecycle}}]-(x) return count(r) as c`

	results := []struct {
		Count int `json:"c"`
	}{}

	qs := &neoism.CypherQuery{
		Statement:  countQuery,
		Parameters: neoism.Props{"contentID": contentID, "lifecycle": "annotations-" + platformVersion},
		Result:     &results,
	}

	conn := getNeoConnection(t)
	err := conn.CypherBatch([]*neoism.CypherQuery{qs})
	assert.NoError(err)
	assert.Equal(1, len(results), "More results found than expected!")
	assert.Equal(1, results[0].Count, "No Relationship with Lifecycle found!")
}

func cleanUp(t *testing.T, contentUUID string, annotationLifecycle string, conceptUUIDs []string) {
	assert := assert.New(t)
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	found, err := annotationsService.Delete(contentUUID, tid, annotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	err = deleteNode(conn, contentUUID)
	assert.NoError(err, "Could not delete content node")

	for _, conceptUUID := range conceptUUIDs {
		err = deleteNode(conn, conceptUUID)
		assert.NoError(err, "Could not delete concept node")
	}
}

func cleanDB(t *testing.T, assert *assert.Assertions) {
	conn := getNeoConnection(t)
	annotationsService = NewCypherAnnotationsService(conn)
	qs := []*neoism.CypherQuery{
		{
			Statement: "MATCH (mc:Thing {uuid: {contentUUID}}) DETACH DELETE mc",
			Parameters: map[string]interface{}{
				"contentUUID": contentUUID,
			},
		},
		{
			Statement: "MATCH (fc:Thing {uuid: {conceptUUID}}) DETACH DELETE fc",
			Parameters: map[string]interface{}{
				"conceptUUID": conceptUUID,
			},
		},
		{
			Statement: "MATCH (fc:Thing {uuid: {secondConceptUUID}}) DETACH DELETE fc",
			Parameters: map[string]interface{}{
				"secondConceptUUID": secondConceptUUID,
			},
		},
		{
			Statement: "MATCH (fc:Thing {uuid: {oldConceptUUID}}) DETACH DELETE fc",
			Parameters: map[string]interface{}{
				"oldConceptUUID": oldConceptUUID,
			},
		},
		{
			Statement: "MATCH (fc:Thing {uuid: {brandUUID}}) DETACH DELETE fc",
			Parameters: map[string]interface{}{
				"brandUUID": brandUUID,
			},
		},
	}

	err := conn.CypherBatch(qs)
	assert.NoError(err)
}

func deleteNode(conn neoutils.NeoConnection, uuid string) error {

	query := &neoism.CypherQuery{
		Statement: `
			MATCH (p:Thing {uuid: {uuid}})
			DELETE p
		`,
		Parameters: map[string]interface{}{
			"uuid": uuid,
		},
	}

	return conn.CypherBatch([]*neoism.CypherQuery{query})
}

func exampleConcepts(uuid string) Annotations {
	return Annotations{exampleConcept(uuid)}
}
