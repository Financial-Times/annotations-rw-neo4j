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
	conceptWithoutAgent = Annotations{Annotation{
		Thing: Thing{ID: getURI(conceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "mentions",
		},
		Provenances: []Provenance{
			{
				Scores: []Score{
					{ScoringSystem: relevanceScoringSystem, Value: 0.9},
					{ScoringSystem: confidenceScoringSystem, Value: 0.8},
				},
				AgentRole: "",
				AtTime:    "",
			},
		},
	}}
	conceptWithPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "isClassifiedBy",
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
	}
	conceptWithAboutPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "about",
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
	}
	conceptWithHasAuthorPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/person/Person",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "hasAuthor",
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
	}
	conceptWithHasContributorPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/person/Person",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "hasContributor",
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
	}

	conceptWithHasDisplayTagPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/person/Person",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "hasDisplayTag",
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
	}

	conceptWithImplicitlyClassifiedByPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/person/Person",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "implicitlyClassifiedBy",
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
	}

	conceptWithHasBrandPredicate = Annotation{
		Thing: Thing{ID: getURI(oldConceptUUID),
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
	}
)

func exampleConcept(uuid string) Annotation {
	return Annotation{
		Thing: Thing{ID: getURI(uuid),
			PrefLabel: "prefLabel",
			Types: []string{
				"http://www.ft.com/ontology/organisation/Organisation",
				"http://www.ft.com/ontology/core/Thing",
				"http://www.ft.com/ontology/concept/Concept",
			},
			Predicate: "mentions",
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
	}
}
