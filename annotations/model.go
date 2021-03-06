package annotations

//Annotations represents a collection of Annotation instances
type Annotations []Annotation

//Annotation is the main struct used to create and return structures
type Annotation struct {
	Thing       Thing        `json:"thing,omitempty"`
	Provenances []Provenance `json:"provenances,omitempty"`
}

//Thing represents a concept being linked to
type Thing struct {
	ID        string   `json:"id,omitempty"`
	PrefLabel string   `json:"prefLabel,omitempty"`
	Types     []string `json:"types,omitempty"`
	Predicate string   `json:"predicate,omitempty"`
}

//Provenance indicates the scores and where they came from
type Provenance struct {
	Scores    []Score `json:"scores,omitempty"`
	AgentRole string  `json:"agentRole,omitempty"`
	AtTime    string  `json:"atTime,omitempty"`
}

//Score represents one of our scores for the annotation
type Score struct {
	ScoringSystem string  `json:"scoringSystem,omitempty"`
	Value         float64 `json:"value,omitempty"`
}

const (
	relevanceScoringSystem  = "http://api.ft.com/scoringsystem/FT-RELEVANCE-SYSTEM"
	confidenceScoringSystem = "http://api.ft.com/scoringsystem/FT-CONFIDENCE-SYSTEM"
)

var relations = map[string]string{
	"mentions":                "MENTIONS",
	"isClassifiedBy":          "IS_CLASSIFIED_BY",
	"implicitlyClassifiedBy":  "IMPLICITLY_CLASSIFIED_BY",
	"about":                   "ABOUT",
	"isPrimarilyClassifiedBy": "IS_PRIMARILY_CLASSIFIED_BY",
	"majorMentions":           "MAJOR_MENTIONS",
	"hasAuthor":               "HAS_AUTHOR",
	"hasContributor":          "HAS_CONTRIBUTOR",
	"hasDisplayTag":           "HAS_DISPLAY_TAG",
	"hasBrand":                "HAS_BRAND",
}
