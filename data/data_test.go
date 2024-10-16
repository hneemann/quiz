package data

import (
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	f, err := os.Open("testdata/Quiz.xml")
	assert.NoError(t, err)

	lecture, err := New(f)
	assert.NoError(t, err)

	task1 := lecture.Chapter[0].Task[0]

	input := make(DataMap)
	input["elBau"] = true
	input["zwei"] = true
	input["drei"] = true
	input["linear"] = true
	input["nlinear"] = true

	result := task1.Validate(input, false)

	assert.Equal(t, 3, len(result))
	assert.True(t, strings.Contains(result["Task"], "gleichzeitig"))
	assert.EqualValues(t, DefaultMessage, result["drei"])
	assert.EqualValues(t, DefaultMessage, result["linear"])

	input = make(DataMap)
	input["elBau"] = true
	input["zwei"] = true
	input["drei"] = false
	input["linear"] = false
	input["nlinear"] = true

	result = task1.Validate(input, false)
	assert.Equal(t, 0, len(result))

	task2 := lecture.Chapter[0].Task[1]
	input = make(DataMap)
	input["func"] = "3*x^2+2"
	result = task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func"] = "2+x^2*3"
	result = task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func"] = "3*x^2+1"
	result = task2.Validate(input, false)
	assert.Equal(t, 1, len(result))

}
