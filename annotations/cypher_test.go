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

var annotationsDriver service

const (
	contentUUID               = "32b089d2-2aae-403d-be6e-877404f586cf"
	conceptUUID               = "a7732a22-3884-4bfe-9761-fef161e41d69"
	secondConceptUUID         = "c834adfa-10c9-4748-8a21-c08537172706"
	oldConceptUUID            = "ad28ddc7-4743-4ed3-9fad-5012b61fb919"
	brandUUID                 = "8e21cbd4-e94b-497a-a43b-5b2309badeb3"
	v2PlatformVersion         = "v2"
	v1PlatformVersion         = "v1"
	pacPlatformVersion        = "pac"
	nextVideoPlatformVersion  = "next-video"
	brightcovePlatformVersion = "brightcove"
	contentLifecycle          = "content"
	v2AnnotationLifecycle     = "annotations-v2"
	v1AnnotationLifecycle     = "annotations-v1"
	pacAnnotationLifecycle    = "annotations-pac"
	tid                       = "transaction_id"
)

func getURI(uuid string) string {
	return fmt.Sprintf("http://api.ft.com/things/%s", uuid)
}

func TestConstraintsApplied(t *testing.T) {
	assert := assert.New(t)
	annotationsDriver = getAnnotationsService(t)
	defer cleanDB(t, assert)

	err := annotationsDriver.Initialise()
	assert.NoError(err)

	testSetupQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}}) SET n :Thing`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	err = annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{testSetupQuery})
	assert.NoError(err, "Error setting up Test data")
	testQuery := &neoism.CypherQuery{
		Statement: `CREATE (n:Thing {uuid:{contentUuid}}) SET n :Thing`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}
	expectErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{testQuery})
	assert.Error(expectErr, "DB constraint is not applied correctly")
}

func TestDeleteRemovesAnnotationsButNotConceptsOrContent(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	annotationsToDelete := exampleConcepts(conceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToDelete), "Failed to write annotation")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, annotationsToDelete)

	deleted, err := annotationsDriver.Delete(contentUUID, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s: %s", contentUUID, err)
	assert.NoError(err, "Error deleting annotation for content uuid %, conceptUUID %s", contentUUID, conceptUUID)

	anns, found, err := annotationsDriver.Read(contentUUID, v2AnnotationLifecycle)

	assert.Equal(Annotations{}, anns, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.False(found, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.NoError(err, "Error trying to find annotation for content %s", contentUUID)

	checkNodeIsStillPresent(contentUUID, t)
	checkNodeIsStillPresent(conceptUUID, t)

	err = deleteNode(annotationsDriver, contentUUID)
	assert.NoError(err, "Error trying to delete content node with uuid %s, err=%v", contentUUID, err)
	err = deleteNode(annotationsDriver, conceptUUID)
	assert.NoError(err, "Error trying to delete concept node with uuid %s, err=%v", conceptUUID, err)
}

func TestWriteFailsWhenNoConceptIDSupplied(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)

	err := annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, conceptWithoutID)
	assert.Error(err, "Should have failed to write annotation")
	_, ok := err.(ValidationError)
	assert.True(ok, "Should have returned a validation error")
}

func TestWriteAllValuesPresent(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, annotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID})
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithoutLifecycle(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
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

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{testSetupQuery})
	assert.NoError(err)

	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")
	checkRelationship(assert, contentUUID, "v2")

	deleted, err := annotationsDriver.Delete(contentUUID, v2AnnotationLifecycle)
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

	readErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithContentLifeCycle(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
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

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{contentQuery})
	assert.NoError(err, "Error c for content uuid %s", contentUUID)

	annotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, annotationsToWrite), "Failed to write annotation")
	checkRelationship(assert, contentUUID, "v2")

	deleted, err := annotationsDriver.Delete(contentUUID, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		Uuid string `json:"b.uuid"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[:IS_CLASSIFIED_BY]->(b:Brand) RETURN b.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"brandUuid":   brandUUID,
		},
		Result: &result,
	}

	readErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesRemoveExistingIsClassifiedForV1TermsAndTheirRelationships(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver := getAnnotationsService(t)

	createContentQuery := &neoism.CypherQuery{
		Statement: `MERGE (c:Content{uuid:{contentUuid}}) SET c :Thing RETURN c.uuid`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{createContentQuery})

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

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{contentQuery})

	assert.NoError(annotationsDriver.Write(contentUUID, v1AnnotationLifecycle, v1PlatformVersion, tid, exampleConcepts(conceptUUID)), "Failed to write annotation")
	found, err := annotationsDriver.Delete(contentUUID, v1AnnotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		Uuid string `json:"b.uuid"`
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

	readErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
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

	readErr = annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})
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

	annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{removeRelationshipQuery})

	err = deleteNode(annotationsDriver, brandUUID)
	assert.NoError(err, "Error trying to delete concept node with uuid %s, err=%v", brandUUID, err)
	err = deleteNode(annotationsDriver, secondConceptUUID)
	assert.NoError(err, "Error trying to delete concept node with uuid %s, err=%v", secondConceptUUID, err)
}

func TestWriteAndReadMultipleAnnotations(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, multiConceptAnnotations), "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, multiConceptAnnotations)
	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, secondConceptUUID})
}

func TestWriteFailsForInvalidPredicate(t *testing.T) {
	annotationsDriver = getAnnotationsService(t)
	err := annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, Annotations{conceptWithInvalidPredicate})
	assert.EqualError(t, err, "Unsupported predicate")
}

func TestIfProvenanceGetsWrittenWithEmptyAgentRoleAndTimeValues(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, conceptWithoutAgent), "Failed to write annotation")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, conceptWithoutAgent)
	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID})
}

// TODO this test can be removed when the special handling for Brightcove videos with annotations-brightcove as lifecycle will be removed (see cypher.go)
func TestNextVideoAnnotationsUpdateDeletesBrightcoveAnnotations(t *testing.T) {
	assert := assert.New(t)
	defer cleanDB(t, assert)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)

	contentQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}})
		 	    MERGE (a:Thing{uuid:{conceptUuid}})
			    CREATE (n)-[rel:MENTIONS{platformVersion:{platformVersion}, lifecycle:{lifecycle}}]->(a)`,
		Parameters: map[string]interface{}{
			"contentUuid":     contentUUID,
			"conceptUuid":     conceptUUID,
			"platformVersion": brightcovePlatformVersion,
			"lifecycle":       brightcoveAnnotationLifecycle,
		},
	}

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{contentQuery})
	assert.NoError(err, "Error creating test data in database.")

	assert.NoError(annotationsDriver.Write(contentUUID, nextVideoAnnotationsLifecycle, nextVideoPlatformVersion, tid, exampleConcepts(conceptUUID)), "Failed to write annotation.")

	result := []struct {
		Lifecycle       string `json:"r.lifecycle"`
		PlatformVersion string `json:"r.platformVersion"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[r]->(b:Thing {uuid:{conceptUuid}}) RETURN r.lifecycle, r.platformVersion`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"conceptUuid": conceptUUID,
		},
		Result: &result,
	}

	readErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})

	assert.NoError(readErr)
	assert.Equal(1, len(result), "Relationships size worng.")

	if len(result) > 0 {
		assert.Equal(nextVideoPlatformVersion, result[0].PlatformVersion, "Platform version wrong.")
		assert.Equal(nextVideoAnnotationsLifecycle, result[0].Lifecycle, "Lifecycle wrong.")
	}
}

// TODO this test can be removed when the special handling for Brightcove videos with annotations-brightcove as lifecycle will be removed (see cypher.go)
func TestNextVideoDeleteCleansAlsoBrightcoveAnnotations(t *testing.T) {
	assert := assert.New(t)
	defer cleanDB(t, assert)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)

	contentQuery := &neoism.CypherQuery{
		Statement: `MERGE (n:Thing {uuid:{contentUuid}})
		 	    MERGE (a:Thing{uuid:{conceptUuid}})
			    CREATE (n)-[rel:MENTIONS{platformVersion:{platformVersion}, lifecycle:{lifecycle}}]->(a)`,
		Parameters: map[string]interface{}{
			"contentUuid":     contentUUID,
			"conceptUuid":     conceptUUID,
			"platformVersion": brightcovePlatformVersion,
			"lifecycle":       brightcoveAnnotationLifecycle,
		},
	}

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{contentQuery})
	assert.NoError(err, "Error creating test data in database.")

	_, err = annotationsDriver.Delete(contentUUID, nextVideoAnnotationsLifecycle)
	assert.NoError(err, "Failed to delete annotation.")

	result := []struct {
		PlatformVersion string `json:"r.platformVersion"`
	}{}

	getContentQuery := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing {uuid:{contentUuid}})-[r]->(b:Thing {uuid:{conceptUuid}}) RETURN r.platformVersion`,
		Parameters: map[string]interface{}{
			"contentUuid": contentUUID,
			"conceptUuid": conceptUUID,
		},
		Result: &result,
	}

	readErr := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{getContentQuery})

	assert.NoError(readErr)
	assert.Empty(result, "Relationship not cleaned.")

	deleteNode(annotationsDriver, contentUUID)
	deleteNode(annotationsDriver, conceptUUID)
}

func TestUpdateWillRemovePreviousAnnotations(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	oldAnnotationsToWrite := exampleConcepts(oldConceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, oldAnnotationsToWrite), "Failed to write annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, oldAnnotationsToWrite)

	updatedAnnotationsToWrite := exampleConcepts(conceptUUID)

	assert.NoError(annotationsDriver.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, tid, updatedAnnotationsToWrite), "Failed to write updated annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, contentUUID, v2AnnotationLifecycle, updatedAnnotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, oldConceptUUID})
}

func TestConnectivityCheck(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	err := annotationsDriver.Check()
	assert.NoError(err, "Unexpected error on connectivity check")
}

func TestCreateAnnotationQuery(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationToWrite := exampleConcept(oldConceptUUID)

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, v2PlatformVersion, v2AnnotationLifecycle)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	params := query.Parameters["annProps"].(map[string]interface{})
	assert.Equal(v2PlatformVersion, params["platformVersion"], fmt.Sprintf("\nExpected: %s\nActual: %s", v2PlatformVersion, params["platformVersion"]))
}

func TestGetRelationshipFromPredicate(t *testing.T) {
	var tests = []struct {
		predicate    string
		relationship string
	}{
		{"mentions", "MENTIONS"},
		{"isClassifiedBy", "IS_CLASSIFIED_BY"},
		{"", "MENTIONS"},
		{"about", "ABOUT"},
		{"hasAuthor", "HAS_AUTHOR"},
	}

	for _, test := range tests {
		actualRelationship, err := getRelationshipFromPredicate(test.predicate)
		assert.NoError(t, err)

		if test.relationship != actualRelationship {
			t.Errorf("\nExpected: %s\nActual: %s", test.relationship, actualRelationship)
		}
	}
}

func TestCreateAnnotationQueryWithPredicate(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationToWrite := conceptWithPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, v2AnnotationLifecycle, v2PlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "IS_CLASSIFIED_BY", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "IS_CLASSIFIED_BY should be inserted instead of MENTIONS")
}

func TestCreateAnnotationQueryWithAboutPredicate(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationToWrite := conceptWithAboutPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, v2AnnotationLifecycle, v2PlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "ABOUT", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "ABOUT should be inserted instead of MENTIONS")
}

func TestCreateAnnotationQueryWithHasAuthorPredicate(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationToWrite := conceptWithHasAuthorPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, v2AnnotationLifecycle, v2PlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "HAS_AUTHOR", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "HAS_AUTHOR should be inserted instead of MENTIONS")
}

func TestCreateAnnotationQueryWithHasContributorPredicate(t *testing.T) {
	assert := assert.New(t)
	annotationToWrite := conceptWithHasContributorPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, pacAnnotationLifecycle, pacPlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "HAS_CONTRIBUTOR", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "HAS_CONTRIBUTOR should be inserted instead of MENTIONS")
}

func TestCreateAnnotationQueryWithHasDisplayTagPredicate(t *testing.T) {
	assert := assert.New(t)
	annotationToWrite := conceptWithHasDisplayTagPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, pacAnnotationLifecycle, pacPlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "HAS_DISPLAY_TAG", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "HAS_DISPLAY_TAG should be inserted instead of MENTIONS")
}

func TestCreateAnnotationQueryWithImplicitlyClassifiedByPredicate(t *testing.T) {
	assert := assert.New(t)
	annotationToWrite := conceptWithImplicitlyClassifiedByPredicate

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, pacAnnotationLifecycle, pacPlatformVersion)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	assert.Contains(query.Statement, "IMPLICITLY_CLASSIFIED_BY", "Relationship name is not inserted!")
	assert.NotContains(query.Statement, "MENTIONS", "IMPLICITLY_CLASSIFIED_BY should be inserted instead of MENTIONS")
}

func getAnnotationsService(t *testing.T) service {
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

	return NewCypherAnnotationsService(db)
}

func readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t *testing.T, contentUUID string, annotationLifecycle string, expectedAnnotations []Annotation) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	storedThings, found, err := annotationsDriver.Read(contentUUID, annotationLifecycle)
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
	}
}

func checkNodeIsStillPresent(uuid string, t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
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

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{query})
	assert.NoError(err, "UnexpectedError")
	assert.True(len(results) == 1, "Didn't find a node")
	assert.Equal(uuid, results[0].UUID, "Did not find correct node")
}

func checkConceptNodeIsStillPresent(uuid string, t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotationsDriver = getAnnotationsService(t)
	results := []struct {
		UUID string `json:"uuid"`
	}{}

	query := &neoism.CypherQuery{
		Statement: `MATCH (n:Thing)<-[IDENTIFIER]-(upp:UPPIdentifier{value:{uuid}}) return n.uuid
		as uuid`,
		Parameters: map[string]interface{}{
			"uuid": uuid,
		},
		Result: &results,
	}

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{query})
	assert.NoError(err, "UnexpectedError")
	assert.True(len(results) == 1, "Didn't find a node")
	assert.Equal(uuid, results[0].UUID, "Did not find correct node")
}

func writeClassifedByRelationship(db neoutils.CypherRunner, contentId string, conceptId string, lifecycle string, t *testing.T, assert *assert.Assertions) {

	var annotateQuery string
	var qs []*neoism.CypherQuery

	if lifecycle == "" {
		annotateQuery = `
                MERGE (content:Thing{uuid:{contentId}})
                MERGE (upp:Identifier:UPPIdentifier{value:{conceptId}})
                MERGE (upp)-[:IDENTIFIES]->(concept:Thing) ON CREATE SET concept.uuid = {conceptId}
                MERGE (content)-[pred:IS_CLASSIFIED_BY {platformVersion:'v1'}]->(concept)
          `
		qs = []*neoism.CypherQuery{
			{
				Statement:  annotateQuery,
				Parameters: neoism.Props{"contentId": contentId, "conceptId": conceptId},
			},
		}
	} else {
		annotateQuery = `
                MERGE (content:Thing{uuid:{contentId}})
                MERGE (upp:Identifier:UPPIdentifier{value:{conceptId}})
                MERGE (upp)-[:IDENTIFIES]->(concept:Thing) ON CREATE SET concept.uuid = {conceptId}
                MERGE (content)-[pred:IS_CLASSIFIED_BY {platformVersion:'v1', lifecycle: {lifecycle}}]->(concept)
          `
		qs = []*neoism.CypherQuery{
			{
				Statement:  annotateQuery,
				Parameters: neoism.Props{"contentId": contentId, "conceptId": conceptId, "lifecycle": lifecycle},
			},
		}

	}

	err := db.CypherBatch(qs)
	assert.NoError(err)
}

func checkRelationship(assert *assert.Assertions, contentID string, platformVersion string) {
	countQuery := `Match (t:Thing {uuid: {contentID}})-[r {lifecycle: {lifecycle}}]-(x) return count(r) as c`

	results := []struct {
		Count int `json:"c"`
	}{}

	qs := &neoism.CypherQuery{
		Statement:  countQuery,
		Parameters: neoism.Props{"contentID": contentID, "lifecycle": "annotations-" + platformVersion},
		Result:     &results,
	}

	err := annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{qs})
	assert.NoError(err)
	assert.Equal(1, len(results), "More results found than expected!")
	assert.Equal(1, results[0].Count, "No Relationship with Lifecycle found!")
}

func cleanUp(t *testing.T, contentUUID string, annotationLifecycle string, conceptUUIDs []string) {
	assert := assert.New(t)
	found, err := annotationsDriver.Delete(contentUUID, annotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	err = deleteNode(annotationsDriver, contentUUID)
	assert.NoError(err, "Could not delete content node")

	for _, conceptUUID := range conceptUUIDs {
		err = deleteNode(annotationsDriver, conceptUUID)
		assert.NoError(err, "Could not delete concept node")
	}
}

func cleanDB(t *testing.T, assert *assert.Assertions) {
	annotationsDriver = getAnnotationsService(t)
	qs := []*neoism.CypherQuery{
		{
			Statement: fmt.Sprintf("MATCH (mc:Thing {uuid: '%v'}) DETACH DELETE mc", contentUUID),
		},
		{
			Statement: fmt.Sprintf("MATCH (fc:Thing {uuid: '%v'}) DETACH DELETE fc", conceptUUID),
		},
		{
			Statement: fmt.Sprintf("MATCH (fc:Thing {uuid: '%v'}) DETACH DELETE fc", secondConceptUUID),
		},
		{
			Statement: fmt.Sprintf("MATCH (fc:Thing {uuid: '%v'}) DETACH DELETE fc", oldConceptUUID),
		},
		{
			Statement: fmt.Sprintf("MATCH (fc:Thing {uuid: '%v'}) DETACH DELETE fc", brandUUID),
		},
	}

	err := annotationsDriver.conn.CypherBatch(qs)
	assert.NoError(err)
}

func deleteNode(annotationsDriver service, uuid string) error {

	query := &neoism.CypherQuery{
		Statement: `
			MATCH (p:Thing {uuid: {uuid}})
			OPTIONAL MATCH (identifier:UPPIdentifier)-[rel:IDENTIFIES]->(p)
			DELETE identifier, rel, p
		`,
		Parameters: map[string]interface{}{
			"uuid": uuid,
		},
	}

	return annotationsDriver.conn.CypherBatch([]*neoism.CypherQuery{query})
}
