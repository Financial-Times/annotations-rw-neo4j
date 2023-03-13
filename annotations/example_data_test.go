package annotations

import "fmt"

const (
	conceptUUID       = "a7732a22-3884-4bfe-9761-fef161e41d69"
	oldConceptUUID    = "ad28ddc7-4743-4ed3-9fad-5012b61fb919"
	secondConceptUUID = "c834adfa-10c9-4748-8a21-c08537172706"
)

func getURI(uuid string) string {
	return fmt.Sprintf("http://api.ft.com/things/%s", uuid)
}

var (
	conceptWithPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/organisation/Organisation",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "isClassifiedBy",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}
	conceptWithAboutPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/organisation/Organisation",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "about",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}
	conceptWithHasAuthorPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/person/Person",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "hasAuthor",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}
	conceptWithHasContributorPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/person/Person",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "hasContributor",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}
	conceptWithHasDisplayTagPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/person/Person",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "hasDisplayTag",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}

	conceptWithImplicitlyClassifiedByPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
		PrefLabel: "prefLabel",
		Types: []string{
			"http://www.ft.com/ontology/person/Person",
			"http://www.ft.com/ontology/core/Thing",
			"http://www.ft.com/ontology/concept/Concept",
		},
		Predicate:       "implicitlyClassifiedBy",
		RelevanceScore:  0.9,
		ConfidenceScore: 0.8,
		AnnotatedBy:     "http://api.ft.com/things/0edd3c31-1fd0-4ef6-9230-8d545be3880a",
		AnnotatedDate:   "2016-01-01T19:43:47.314Z",
	}

	conceptWithHasBrandPredicate = Annotation{
		ID:        getURI(oldConceptUUID),
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
	}
)

func exampleConcept(uuid string) Annotation {
	return Annotation{
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
	}
}
