package ontology

// Annotations represents a collection of Annotation instances
type Annotations []Annotation

// Annotation is the main struct containing the annotations attributes
type Annotation struct {
	ID                 string   `json:"id,omitempty"`
	PrefLabel          string   `json:"prefLabel,omitempty"`
	Types              []string `json:"types,omitempty"`
	Predicate          string   `json:"predicate,omitempty"`
	RelevanceScore     float64  `json:"relevanceScore,omitempty"`
	ConfidenceScore    float64  `json:"confidenceScore,omitempty"`
	AnnotatedBy        string   `json:"annotatedBy,omitempty"`
	AnnotatedDate      string   `json:"annotatedDate,omitempty"`
	AnnotatedDateEpoch int64    `json:"annotatedDateEpoch,omitempty"`
}

var Relations = map[string]string{
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
