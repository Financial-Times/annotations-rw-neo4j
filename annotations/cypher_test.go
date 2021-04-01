package annotations

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Financial-Times/go-logger"
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
	logger.InitDefaultLogger("annotations-rw")
	annotationToWrite := exampleConcept(oldConceptUUID)

	query, err := createAnnotationQuery(contentUUID, annotationToWrite, v2PlatformVersion, v2AnnotationLifecycle)
	assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
	params := query.Parameters["annProps"].(map[string]interface{})
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
			logger.InitDefaultLogger("annotations-rw")
			query, err := createAnnotationQuery(contentUUID, test.annotationToWrite, test.platformVersion, test.lifecycle)

			assert.NoError(err, "Cypher query for creating annotations couldn't be created.")
			assert.Contains(query.Statement, test.relationship, "Relationship name is not inserted!")
			assert.NotContains(query.Statement, "MENTIONS", fmt.Sprintf("%s should be inserted instead of MENTIONS", test.relationship))
		})
	}
}

func TestCreateAnnotationQueryWithoutPredicate(t *testing.T) {
	assert := assert.New(t)
	logger.InitDefaultLogger("annotations-rw")
	annotation := exampleConcept(oldConceptUUID)
	annotation.Thing.Predicate = ""

	_, err := createAnnotationQuery(contentUUID, annotation, v2PlatformVersion, v2AnnotationLifecycle)
	assert.True(errors.Is(err, UnsupportedPredicateErr), "Creating annotation without predicate is not allowed.")
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
		actualRelationship, err := getRelationshipFromPredicate(test.predicate)
		assert.NoError(t, err)

		if test.relationship != actualRelationship {
			t.Errorf("\nExpected: %s\nActual: %s", test.relationship, actualRelationship)
		}
	}
}
