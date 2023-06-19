package ontology

import (
	"github.com/Financial-Times/go-logger/v2"
	"github.com/Financial-Times/upp-content-validator-kit/v2/schema"
	"github.com/Financial-Times/upp-content-validator-kit/v2/validator"
)

type SchemaValidator struct {
	schemaReaderConfig  *schema.ReaderConfig
	schemaReader        *schema.Reader
	jsonValidatorConfig *validator.JSONValidationConfig
	jsonValidator       *validator.JSONValidator
	log                 *logger.UPPLogger
}

func NewSchemaValidator(log *logger.UPPLogger) *SchemaValidator {
	return &SchemaValidator{log: log}
}

func (v *SchemaValidator) GetJSONValidator() *validator.JSONValidator {
	if v.jsonValidator == nil {
		schemaReader := v.GetSchemaReader()
		conf := v.GetJSONValidatorConfig()
		jsonValidator, err := validator.NewJSONValidator(conf.JSONSchemasName, schemaReader)
		if err != nil {
			v.log.WithError(err).Fatalf("unable to create JSON validator")
		}
		v.jsonValidator = jsonValidator
	}
	return v.jsonValidator
}

func (v *SchemaValidator) GetSchemaReader() *schema.Reader {
	if v.schemaReader == nil {
		conf := v.GetSchemaReaderConfig()
		v.schemaReader = schema.NewReader(conf.SchemaFilesPath)
	}
	return v.schemaReader
}

func (v *SchemaValidator) GetSchemaReaderConfig() *schema.ReaderConfig {
	if v.schemaReaderConfig == nil {
		conf, err := schema.NewSchemaReaderConfig()
		if err != nil {
			v.log.WithError(err).Fatalf("cannot load schema readter config")
		}
		v.schemaReaderConfig = conf
	}
	return v.schemaReaderConfig
}

func (v *SchemaValidator) GetJSONValidatorConfig() *validator.JSONValidationConfig {
	if v.jsonValidatorConfig == nil {
		conf, err := validator.NewJSONValidationConfig()
		if err != nil {
			v.log.WithError(err).Fatalf("could not load JSON validator conf")
		}
		v.jsonValidatorConfig = conf
	}
	return v.jsonValidatorConfig
}
