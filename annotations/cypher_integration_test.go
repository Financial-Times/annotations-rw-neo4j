//go:build integration
// +build integration

package annotations

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/Financial-Times/annotations-rw-neo4j/v4/ontology"

	cmneo4j "github.com/Financial-Times/cm-neo4j-driver"
	logger "github.com/Financial-Times/go-logger/v2"
	"github.com/stretchr/testify/assert"
)

const (
	brandUUID                     = "8e21cbd4-e94b-497a-a43b-5b2309badeb3"
	PACPlatformVersion            = "pac"
	nextVideoPlatformVersion      = "next-video"
	nextVideoAnnotationsLifecycle = "next-video"
	contentLifecycle              = "content"
	PACAnnotationLifecycle        = "annotations-pac"
	apiHost                       = "http://api.ft.com"
	v2AnnotationLifecycle         = "annotations-v2"
	v2PlatformVersion             = "v2"
	contentUUID                   = "32b089d2-2aae-403d-be6e-877404f586cf"
	oldConceptUUID                = "ad28ddc7-4743-4ed3-9fad-5012b61fb919"
	conceptUUID                   = "a7732a22-3884-4bfe-9761-fef161e41d69"
	secondConceptUUID             = "c834adfa-10c9-4748-8a21-c08537172706"
)

func TestConstraintsApplied(t *testing.T) {
	t.Skip("Skip, because the driver doesn't support EnsureConstraints/Indexes for Neo4j less than 4.x")

	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	defer cleanDB(t, assert)

	err = annotationsService.Initialise()
	assert.NoError(err, "creating cypher annotations service failed")

	testSetupQuery := &cmneo4j.Query{
		Cypher: `MERGE (n:Thing {uuid:$contentUuid}) SET n :Thing`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	err = driver.Write(testSetupQuery)
	assert.NoError(err, "Error setting up Test data")
	testQuery := &cmneo4j.Query{
		Cypher: `CREATE (n:Thing {uuid:$contentUuid}) SET n :Thing`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}
	expectErr := driver.Write(testQuery)
	assert.Error(expectErr, "DB constraint is not applied correctly")
}

func TestWriteFailsWhenNoConceptIDSupplied(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")

	conceptWithoutID := ontology.Annotations{ontology.Annotation{
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/organisation/Organisation",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}}

	_, err = annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, conceptWithoutID))
	assert.Error(err, "Should have failed to write annotation")
}

func TestDeleteRemovesAnnotationsButNotConceptsOrContent(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	annotationsToDelete := exampleConcepts(conceptUUID)

	bookmark, err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, annotationsToDelete))
	assert.NoError(err, "Failed to write annotation")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, annotationsService, contentUUID, v2AnnotationLifecycle, bookmark, annotationsToDelete)

	deleted, bookmark, err := annotationsService.Delete(contentUUID, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s: %s", contentUUID, err)
	assert.NoError(err, "Error deleting annotation for content uuid %, conceptUUID %s", contentUUID, conceptUUID)

	anns, found, err := annotationsService.Read(contentUUID, bookmark, v2AnnotationLifecycle)

	assert.Equal(ontology.Annotations{}, anns, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.False(found, "Found annotation for content %s when it should have been deleted", contentUUID)
	assert.NoError(err, "Error trying to find annotation for content %s", contentUUID)

	checkNodeIsStillPresent(contentUUID, t)
	checkNodeIsStillPresent(conceptUUID, t)

	err = deleteNode(driver, contentUUID)
	assert.NoError(err, "Error trying to delete content node with uuid %s, err=%v", contentUUID, err)
	err = deleteNode(driver, conceptUUID)
	assert.NoError(err, "Error trying to delete concept node with uuid %s, err=%v", conceptUUID, err)
}

func TestWriteAllValuesPresent(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	annotationsToWrite := exampleConcepts(conceptUUID)

	bookmark, err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, annotationsToWrite))
	assert.NoError(err, "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, annotationsService, contentUUID, v2AnnotationLifecycle, bookmark, annotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID})
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithoutLifecycle(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	defer cleanDB(t, assert)

	testSetupQuery := &cmneo4j.Query{
		Cypher: `MERGE (n:Thing {uuid:$contentUuid}) SET n :Thing
		MERGE (b:Brand{uuid:$brandUuid}) SET b :Concept:Thing
		CREATE (n)-[rel:IS_CLASSIFIED_BY{platformVersion:$platformVersion}]->(b)`,
		Params: map[string]interface{}{
			"contentUuid":     contentUUID,
			"brandUuid":       brandUUID,
			"platformVersion": v2PlatformVersion,
		},
	}

	err = driver.Write(testSetupQuery)
	assert.NoError(err, "creating cypher annotations service failed")

	annotationsToWrite := exampleConcepts(conceptUUID)

	_, err = annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, annotationsToWrite))
	assert.NoError(err, "Failed to write annotation")
	checkRelationship(t, assert, contentUUID, "v2")

	deleted, _, err := annotationsService.Delete(contentUUID, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	getContentQuery := &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$contentUuid})-[:IS_CLASSIFIED_BY]->(b:Brand {uuid:$brandUuid}) RETURN b.uuid`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
			"brandUuid":   brandUUID,
		},
		Result: &result,
	}

	readErr := driver.Read(getContentQuery)
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesNotRemoveExistingIsClassifiedByBrandRelationshipsWithContentLifeCycle(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	defer cleanDB(t, assert)

	contentQuery := &cmneo4j.Query{
		Cypher: `MERGE (n:Thing {uuid:$contentUuid}) SET n :Thing
		MERGE (b:Brand{uuid:$brandUuid}) SET b :Concept:Thing
		CREATE (n)-[rel:IS_CLASSIFIED_BY{platformVersion:$platformVersion, lifecycle: $lifecycle}]->(b)`,
		Params: map[string]interface{}{
			"contentUuid":     contentUUID,
			"brandUuid":       brandUUID,
			"platformVersion": v2PlatformVersion,
			"lifecycle":       contentLifecycle,
		},
	}

	err = driver.Write(contentQuery)
	assert.NoError(err, "Error c for content uuid %s", contentUUID)

	annotationsToWrite := exampleConcepts(conceptUUID)

	_, err = annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, annotationsToWrite))
	assert.NoError(err, "Failed to write annotation")
	checkRelationship(t, assert, contentUUID, "v2")

	deleted, _, err := annotationsService.Delete(contentUUID, v2AnnotationLifecycle)
	assert.True(deleted, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	getContentQuery := &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$contentUuid})-[:IS_CLASSIFIED_BY]->(b:Brand{uuid:$brandUuid}) RETURN b.uuid`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
			"brandUuid":   brandUUID,
		},
		Result: &result,
	}

	readErr := driver.Read(getContentQuery)
	assert.NoError(readErr)
	assert.NotEmpty(result)
}

func TestWriteDoesRemoveExistingIsClassifiedForPACTermsAndTheirRelationships(t *testing.T) {
	assert := assert.New(t)

	defer cleanDB(t, assert)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")

	createContentQuery := &cmneo4j.Query{
		Cypher: `MERGE (c:Content{uuid:$contentUuid}) SET c :Thing RETURN c.uuid`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
		},
	}

	assert.NoError(driver.Write(createContentQuery))

	contentQuery := &cmneo4j.Query{
		Cypher: `MERGE (n:Thing {uuid:$contentUuid})
		 	    MERGE (a:Thing{uuid:$conceptUUID})
			    CREATE (n)-[rel1:MENTIONS{lifecycle:"annotations-v2"}]->(a)
			    MERGE (b:Thing{uuid:$secondConceptUUID})
			    CREATE (n)-[rel2:IS_CLASSIFIED_BY{lifecycle:"annotations-pac"}]->(b)`,
		Params: map[string]interface{}{
			"contentUuid":       contentUUID,
			"conceptUUID":       conceptUUID,
			"secondConceptUUID": secondConceptUUID,
		},
	}

	assert.NoError(driver.Write(contentQuery))

	_, err = annotationsService.Write(contentUUID, PACAnnotationLifecycle, PACPlatformVersion, convertAnnotations(t, exampleConcepts(conceptUUID)))
	assert.NoError(err, "Failed to write annotation")
	found, bookmark, err := annotationsService.Delete(contentUUID, PACAnnotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	result := []struct {
		UUID string `json:"b.uuid"`
	}{}

	//CHECK THAT ALL THE PAC annotations are deleted
	getContentQuery := &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$contentUuid})-[r]->(b:Thing) where r.lifecycle=$lifecycle RETURN b.uuid`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
			"lifecycle":   PACAnnotationLifecycle,
		},
		Result: &result,
	}

	bookmark, readErr := driver.ReadMultiple([]*cmneo4j.Query{getContentQuery}, []string{bookmark})
	assert.True(errors.Is(readErr, cmneo4j.ErrNoResultsFound), "ErrNoResultsFound is expected")
	assert.Empty(result)

	//CHECK THAT V2 annotations were not deleted
	getContentQuery = &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$contentUuid})-[r]->(b:Thing) where r.lifecycle=$lifecycle RETURN b.uuid`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
			"lifecycle":   v2AnnotationLifecycle,
		},
		Result: &result,
	}

	_, readErr = driver.ReadMultiple([]*cmneo4j.Query{getContentQuery}, []string{bookmark})
	assert.NoError(readErr)
	assert.NotEmpty(result)

	//Delete v2 annotations
	removeRelationshipQuery := &cmneo4j.Query{
		Cypher: `
			MATCH (b:Thing {uuid:$conceptUUID})<-[rel]-(t:Thing)
			where rel.platformVersion = "v2"
			DELETE rel
		`,
		Params: map[string]interface{}{
			"conceptUUID": conceptUUID,
		},
	}

	assert.NoError(driver.Write(removeRelationshipQuery))
}

func TestWriteAndReadMultipleAnnotations(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")

	multiConceptAnnotations := ontology.Annotations{
		ontology.Annotation{
			ID:        getURI(conceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/product/Brand",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate:       "hasBrand",
			RelevanceScore:  0.9,
			ConfidenceScore: 0.8,
			AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
			AnnotatedDate:   "2016-01-01T19:43:47.314Z",
		},
		ontology.Annotation{
			ID:        getURI(secondConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate:       "mentions",
			RelevanceScore:  0.4,
			ConfidenceScore: 0.5,
			AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
			AnnotatedDate:   "2016-01-01T19:43:47.314Z",
		},
	}

	bookmark, err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, multiConceptAnnotations))
	assert.NoError(err, "Failed to write annotation")

	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, annotationsService, contentUUID, v2AnnotationLifecycle, bookmark, multiConceptAnnotations)
	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, secondConceptUUID})
}

func TestNextVideoAnnotationsUpdatesAnnotations(t *testing.T) {
	assert := assert.New(t)
	defer cleanDB(t, assert)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")

	contentQuery := &cmneo4j.Query{
		Cypher: `CREATE (n:Thing {uuid:$contentUuid})
		 	    CREATE (a:Thing{uuid:$conceptUuid})
			    CREATE (n)-[rel:MENTIONS{platformVersion:$platformVersion, lifecycle:$lifecycle}]->(a)`,
		Params: map[string]interface{}{
			"contentUuid":     contentUUID,
			"conceptUuid":     conceptUUID,
			"platformVersion": nextVideoAnnotationsLifecycle,
			"lifecycle":       nextVideoAnnotationsLifecycle,
		},
	}

	err = driver.Write(contentQuery)
	assert.NoError(err, "Error creating test data in database.")

	_, err = annotationsService.Write(contentUUID, nextVideoAnnotationsLifecycle, nextVideoPlatformVersion, convertAnnotations(t, exampleConcepts(secondConceptUUID)))
	assert.NoError(err, "Failed to write annotation.")

	result := []struct {
		Lifecycle       string `json:"r.lifecycle"`
		PlatformVersion string `json:"r.platformVersion"`
	}{}

	getContentQuery := &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$contentUuid})-[r]->(b:Thing {uuid:$conceptUuid}) RETURN r.lifecycle, r.platformVersion`,
		Params: map[string]interface{}{
			"contentUuid": contentUUID,
			"conceptUuid": secondConceptUUID,
		},
		Result: &result,
	}

	readErr := driver.Read(getContentQuery)

	assert.NoError(readErr)
	assert.Equal(1, len(result), "Relationships size worng.")

	if len(result) > 0 {
		assert.Equal(nextVideoPlatformVersion, result[0].PlatformVersion, "Platform version wrong.")
		assert.Equal(nextVideoAnnotationsLifecycle, result[0].Lifecycle, "Lifecycle wrong.")
	}
}

func TestUpdateWillRemovePreviousAnnotations(t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")
	oldAnnotationsToWrite := exampleConcepts(oldConceptUUID)

	bookmark, err := annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, oldAnnotationsToWrite))
	assert.NoError(err, "Failed to write annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, annotationsService, contentUUID, v2AnnotationLifecycle, bookmark, oldAnnotationsToWrite)

	updatedAnnotationsToWrite := exampleConcepts(conceptUUID)

	bookmark, err = annotationsService.Write(contentUUID, v2AnnotationLifecycle, v2PlatformVersion, convertAnnotations(t, updatedAnnotationsToWrite))
	assert.NoError(err, "Failed to write updated annotations")
	readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t, annotationsService, contentUUID, v2AnnotationLifecycle, bookmark, updatedAnnotationsToWrite)

	cleanUp(t, contentUUID, v2AnnotationLifecycle, []string{conceptUUID, oldConceptUUID})
}

func getNeo4jDriver(t *testing.T) *cmneo4j.Driver {
	t.Helper()

	url := os.Getenv("NEO4J_TEST_URL")
	if url == "" {
		url = "bolt://localhost:7687"
	}

	log := logger.NewUPPLogger("annotations-rw-neo4j-cm-neo4j-driver", "ERROR")
	driver, err := cmneo4j.NewDefaultDriver(url, log)

	assert.NoError(t, err, "Unexpected error when creating a new driver")

	return driver
}

// nolint:all
func readAnnotationsForContentUUIDAndCheckKeyFieldsMatch(t *testing.T, svc Service, contentUUID, annotationLifecycle, bookmark string, expectedAnnotations []ontology.Annotation) {
	assert := assert.New(t)
	storedThings, found, err := svc.Read(contentUUID, bookmark, annotationLifecycle)
	storedAnnotations := storedThings.(*[]ontology.Annotation)

	assert.NoError(err, "Error finding annotations for contentUUID %s", contentUUID)
	assert.True(found, "Didn't find annotations for contentUUID %s", contentUUID)
	assert.Equal(len(expectedAnnotations), len(*storedAnnotations), "Didn't get the same number of annotations")

	for idx, storedAnnotation := range *storedAnnotations {
		expectedAnnotation := expectedAnnotations[idx]
		// In annotations write, we don't store anything other than ID for the concept (so type will only be 'Thing' and pref label will not
		// be present UNLESS the concept has been written by some other system)
		assert.Equal(expectedAnnotation.ID, storedAnnotation.ID, "ID is not the same")
		expectedPredicate := getRelationshipFromPredicate(expectedAnnotation.Predicate)
		assert.Equal(expectedPredicate, storedAnnotation.Predicate, "Predicates are not the same")
		assert.Equal(expectedAnnotation.RelevanceScore, storedAnnotation.RelevanceScore, "Relevance score is not the same")
		assert.Equal(expectedAnnotation.ConfidenceScore, storedAnnotation.ConfidenceScore, "Confidence score is not the same")
		assert.Equal(expectedAnnotation.AnnotatedBy, storedAnnotation.AnnotatedBy, "AnnotatedBy is not the same")
		assert.Equal(expectedAnnotation.AnnotatedDate, storedAnnotation.AnnotatedDate, "AnnotatedDate is not the same")
	}
}

func checkNodeIsStillPresent(uuid string, t *testing.T) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	results := []struct {
		UUID string `json:"uuid"`
	}{}

	query := &cmneo4j.Query{
		Cypher: `MATCH (n:Thing {uuid:$uuid}) return n.uuid
		as uuid`,
		Params: map[string]interface{}{
			"uuid": uuid,
		},
		Result: &results,
	}

	err := driver.Read(query)
	assert.NoError(err, "UnexpectedError")
	assert.True(len(results) == 1, "Didn't find a node")
	assert.Equal(uuid, results[0].UUID, "Did not find correct node")
}

func checkRelationship(t *testing.T, assert *assert.Assertions, contentID string, platformVersion string) {
	countQuery := `Match (t:Thing {uuid: $contentID})-[r {lifecycle: $lifecycle}]-(x) return count(r) as c`

	results := []struct {
		Count int `json:"c"`
	}{}

	qs := &cmneo4j.Query{
		Cypher: countQuery,
		Params: map[string]interface{}{
			"contentID": contentID,
			"lifecycle": "annotations-" + platformVersion,
		},
		Result: &results,
	}

	driver := getNeo4jDriver(t)
	err := driver.Read(qs)
	assert.NoError(err, "creating cypher annotations service failed")
	assert.Equal(1, len(results), "More results found than expected!")
	assert.Equal(1, results[0].Count, "No Relationship with Lifecycle found!")
}

func cleanUp(t *testing.T, contentUUID string, annotationLifecycle string, conceptUUIDs []string) {
	assert := assert.New(t)
	driver := getNeo4jDriver(t)
	annotationsService, err := NewCypherAnnotationsService(driver, apiHost)
	assert.NoError(err, "creating cypher annotations service failed")

	found, _, err := annotationsService.Delete(contentUUID, annotationLifecycle)
	assert.True(found, "Didn't manage to delete annotations for content uuid %s", contentUUID)
	assert.NoError(err, "Error deleting annotations for content uuid %s", contentUUID)

	err = deleteNode(driver, contentUUID)
	assert.NoError(err, "Could not delete content node")

	for _, conceptUUID := range conceptUUIDs {
		err = deleteNode(driver, conceptUUID)
		assert.NoError(err, "Could not delete concept node")
	}
}

func cleanDB(t *testing.T, assert *assert.Assertions) {
	driver := getNeo4jDriver(t)

	qs := []*cmneo4j.Query{
		{
			Cypher: "MATCH (mc:Thing {uuid: $contentUUID}) DETACH DELETE mc",
			Params: map[string]interface{}{
				"contentUUID": contentUUID,
			},
		},
		{
			Cypher: "MATCH (fc:Thing {uuid: $conceptUUID}) DETACH DELETE fc",
			Params: map[string]interface{}{
				"conceptUUID": conceptUUID,
			},
		},
		{
			Cypher: "MATCH (fc:Thing {uuid: $secondConceptUUID}) DETACH DELETE fc",
			Params: map[string]interface{}{
				"secondConceptUUID": secondConceptUUID,
			},
		},
		{
			Cypher: "MATCH (fc:Thing {uuid: $oldConceptUUID}) DETACH DELETE fc",
			Params: map[string]interface{}{
				"oldConceptUUID": oldConceptUUID,
			},
		},
		{
			Cypher: "MATCH (fc:Thing {uuid: $brandUUID}) DETACH DELETE fc",
			Params: map[string]interface{}{
				"brandUUID": brandUUID,
			},
		},
	}

	err := driver.Write(qs...)
	assert.NoError(err, "creating cypher annotations service failed")
}

func deleteNode(driver *cmneo4j.Driver, uuid string) error {
	query := &cmneo4j.Query{
		Cypher: `
			MATCH (p:Thing {uuid: $uuid})
			DELETE p
		`,
		Params: map[string]interface{}{
			"uuid": uuid,
		},
	}

	return driver.Write(query)
}

func exampleConcepts(uuid string) ontology.Annotations {
	return ontology.Annotations{
		ontology.Annotation{
			ID:        getURI(uuid),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate:       "mentions",
			RelevanceScore:  0.9,
			ConfidenceScore: 0.8,
			AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
			AnnotatedDate:   "2016-01-01T19:43:47.314Z",
		},
	}
}

func getURI(uuid string) string {
	return fmt.Sprintf("http://api.ft.com/things/%s", uuid)
}

func convertAnnotations(t *testing.T, anns ontology.Annotations) []interface{} {
	var annSlice []interface{}
	for _, ann := range anns {
		var annMap map[string]interface{}
		data, err := json.Marshal(ann)
		if err != nil {
			t.Fatal(err)
		}
		err = json.Unmarshal(data, &annMap)
		if err != nil {
			t.Fatal(err)
		}
		annSlice = append(annSlice, annMap)
	}
	return annSlice
}

func getRelationshipFromPredicate(predicate string) string {
	r, ok := ontology.Relations[extractPredicateFromURI(predicate)]
	if !ok {
		return ""
	}
	return r
}

func extractPredicateFromURI(uri string) string {
	_, result := path.Split(uri)
	return result
}
