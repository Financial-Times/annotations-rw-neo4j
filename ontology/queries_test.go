package ontology

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	contentUUID            = "32b089d2-2aae-403d-be6e-877404f586cf"
	v2PlatformVersion      = "v2"
	pacPlatformVersion     = "pac"
	v2AnnotationLifecycle  = "annotations-v2"
	pacAnnotationLifecycle = "annotations-pac"
)

func TestCreateAnnotationQuery(t *testing.T) {
	assert := assert.New(t)
	annotationToWrite := exampleConcept(oldConceptUUID)

	query, err := CreateAnnotationQuery(contentUUID, convertAnnotationToMap(t, annotationToWrite), v2PlatformVersion, v2AnnotationLifecycle)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	params := query.Params["annProps"].(map[string]interface{})
	assert.Equal(v2PlatformVersion, params["platformVersion"], fmt.Sprintf("\nExpected: %s\nActual: %s", v2PlatformVersion, params["platformVersion"]))
}

func TestCreateAnnotationQueryWithPredicate(t *testing.T) {
	testCases := []struct {
		name              string
		relationship      string
		annotationToWrite Annotation
		lifecycle         string
		platformVersion   string
	}{
		{
			name:              "isClassifiedBy",
			relationship:      "IS_CLASSIFIED_BY",
			annotationToWrite: conceptWithPredicate,
			platformVersion:   v2PlatformVersion,
			lifecycle:         v2AnnotationLifecycle,
		},
		{
			name:              "about",
			relationship:      "ABOUT",
			annotationToWrite: conceptWithAboutPredicate,
			platformVersion:   v2PlatformVersion,
			lifecycle:         v2AnnotationLifecycle,
		},
		{
			name:              "hasAuthor",
			relationship:      "HAS_AUTHOR",
			annotationToWrite: conceptWithHasAuthorPredicate,
			platformVersion:   v2PlatformVersion,
			lifecycle:         v2AnnotationLifecycle,
		},
		{
			name:              "hasContributor",
			relationship:      "HAS_CONTRIBUTOR",
			annotationToWrite: conceptWithHasContributorPredicate,
			platformVersion:   pacPlatformVersion,
			lifecycle:         pacAnnotationLifecycle,
		},
		{
			name:              "hasDisplayTag",
			relationship:      "HAS_DISPLAY_TAG",
			annotationToWrite: conceptWithHasDisplayTagPredicate,
			platformVersion:   pacPlatformVersion,
			lifecycle:         pacAnnotationLifecycle,
		},
		{
			name:              "implicitlyClassifiedBy",
			relationship:      "IMPLICITLY_CLASSIFIED_BY",
			annotationToWrite: conceptWithImplicitlyClassifiedByPredicate,
			platformVersion:   pacPlatformVersion,
			lifecycle:         pacAnnotationLifecycle,
		},
		{
			name:              "hasBrand",
			relationship:      "HAS_BRAND",
			annotationToWrite: conceptWithHasBrandPredicate,
			platformVersion:   pacPlatformVersion,
			lifecycle:         pacAnnotationLifecycle,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)
			query, err := CreateAnnotationQuery(contentUUID, convertAnnotationToMap(t, test.annotationToWrite), test.platformVersion, test.lifecycle)

			assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
			assert.Contains(query.Cypher, test.relationship, "Relationship name is not inserted!")
			assert.NotContains(query.Cypher, "MENTIONS", fmt.Sprintf("%s should be inserted instead of MENTIONS", test.relationship))
		})
	}
}

func TestGetRelationshipFromPredicate(t *testing.T) {
	var tests = []struct {
		predicate    string
		relationship string
	}{
		{"mentions", "MENTIONS"},
		{"isClassifiedBy", "IS_CLASSIFIED_BY"},
		{"implicitlyClassifiedBy", "IMPLICITLY_CLASSIFIED_BY"},
		{"about", "ABOUT"},
		{"isPrimarilyClassifiedBy", "IS_PRIMARILY_CLASSIFIED_BY"},
		{"majorMentions", "MAJOR_MENTIONS"},
		{"hasAuthor", "HAS_AUTHOR"},
		{"hasContributor", "HAS_CONTRIBUTOR"},
		{"hasDisplayTag", "HAS_DISPLAY_TAG"},
		{"hasBrand", "HAS_BRAND"},
	}

	for _, test := range tests {
		actualRelationship := getRelationshipFromPredicate(test.predicate)
		if test.relationship != actualRelationship {
			t.Errorf("\nExpected: %s\nActual: %s", test.relationship, actualRelationship)
		}
	}
}

func convertAnnotationToMap(t *testing.T, ann Annotation) map[string]interface{} {
	var annMap map[string]interface{}
	data, err := json.Marshal(ann)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(data, &annMap)
	if err != nil {
		t.Fatal(err)
	}
	return annMap
}
