package data

import (
	"github.com/hneemann/parser2/value"
	"github.com/stretchr/testify/assert"
	"os"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	f, err := os.Open("testdata/elektronik/elektronik.xml")
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

	assert.Equal(t, 1, len(result))
	assert.True(t, strings.Contains(result["_task_"], "nicht richtig"))

	input = make(DataMap)
	input["elBau"] = true
	input["zwei"] = true
	input["drei"] = false
	input["linear"] = false
	input["nlinear"] = true

	result = task1.Validate(input, false)
	assert.Equal(t, 0, len(result))

	task2 := lecture.Chapter[0].Task[3]
	input = make(DataMap)
	input["func"] = "IS*exp(UD/UT)"
	result = task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func"] = "exp(UD/UT)*IS"
	result = task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func"] = "3*x^2+1"
	result = task2.Validate(input, false)
	assert.Equal(t, 1, len(result))
	assert.EqualValues(t, "", result["_task_"])
}

func TestParser(t *testing.T) {
	test := []struct {
		expr   string
		result value.Value
	}{
		{"1+2", value.Int(3)},
		{"parseFunc(\"1/2\",[]).eval([])", value.Float(0.5)},
		{"parseFunc(\"x+2\",[\"x\"]).eval([1])", value.Float(3)},
		{"parseFunc(\"x+2\",[\"x\"]).varUsages()", value.Int(1)},
		{"parseFunc(\"x+x\",[\"x\"]).varUsages()", value.Int(2)},
		{"parseFunc(\"sin(x)\",[\"x\"]).varUsages()", value.Int(1)},
		{"cmpFunc(\"2*x\",\"x+x\",[\"x\"],[[1],[2],[3]])", value.Bool(true)},
		{"cmpFunc(\"2*x\",\"x+x+1\",[\"x\"],[[1],[2],[3]])", value.Bool(false)},
	}

	input := make(DataMap)
	input["a"] = 1
	for _, tst := range test {
		t.Run(tst.expr, func(t *testing.T) {
			f, err := myParser.Generate(tst.expr, "a")
			assert.NoError(t, err)
			if f == nil {
				r, err := f.Eval(value.NewMap(input))
				assert.NoError(t, err)
				assert.Equal(t, tst.result, r)
			}
		})
	}
}

func Test_cleanupExpression(t *testing.T) {
	tests := []struct {
		raw   string
		clean string
	}{
		{raw: "2x", clean: "2*x"},
		{raw: "2x²", clean: "2*x^2"},
		{raw: "2x³", clean: "2*x^3"},
		{raw: "2x⁴", clean: "2*x^4"},
		{raw: "2x³+x²+x", clean: "2*x^3+x^2+x"},
		{raw: "2(x+1)", clean: "2*(x+1)"},
		{raw: "2sin(x)", clean: "2*sin(x)"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			assert.Equalf(t, tt.clean, cleanupExpression(tt.raw), "cleanupExpression(%v)", tt.raw)
		})
	}
}
