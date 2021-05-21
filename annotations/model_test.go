package annotations

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnMarshallingAnnotation(t *testing.T) {
	assert := assert.New(t)
	annotations := Annotations{}
	jason, err := ioutil.ReadFile("examplePutBody.json")
	assert.NoError(err, "Unexpected error")

	err = json.Unmarshal([]byte(jason), &annotations)
	assert.NoError(err, "Unexpected error")
}
