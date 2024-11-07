package data

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestFromExpParser(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "x", want: "<mi>x</mi>"},
		{input: "x+1", want: "<mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow>"},
		{input: "x+1.5", want: "<mrow><mi>x</mi><mo>+</mo><mn>1.5</mn></mrow>"},
		{input: "a+b+c", want: "<mrow><mi>a</mi><mo>+</mo><mi>b</mi><mo>+</mo><mi>c</mi></mrow>"},
		{input: "a-b+c", want: "<mrow><mi>a</mi><mo>-</mo><mi>b</mi><mo>+</mo><mi>c</mi></mrow>"},
		{input: "a+b-c", want: "<mrow><mi>a</mi><mo>+</mo><mi>b</mi><mo>-</mo><mi>c</mi></mrow>"},
		{input: "a-b-c", want: "<mrow><mi>a</mi><mo>-</mo><mi>b</mi><mo>-</mo><mi>c</mi></mrow>"},
		{input: "(a+b)*c", want: "<mrow><mo>(</mo><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mo>)</mo><mo>*</mo><mi>c</mi></mrow>"},
		{input: "a*(b+c)", want: "<mrow><mi>a</mi><mo>*</mo><mo>(</mo><mrow><mi>b</mi><mo>+</mo><mi>c</mi></mrow><mo>)</mo></mrow>"},
		{input: "a-(b+c)", want: "<mrow><mi>a</mi><mo>-</mo><mo>(</mo><mrow><mi>b</mi><mo>+</mo><mi>c</mi></mrow><mo>)</mo></mrow>"},
		{input: "a/b", want: "<mfrac><mi>a</mi><mi>b</mi></mfrac>"},
		{input: "(a+b)/(b+1)", want: "<mfrac><mi>a</mi><mi>b</mi></mfrac>"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			a, err := floatParser.GetParser().Parse(tt.input)
			assert.NoError(t, err)
			ml := MathMlFromAST(a)

			sb := strings.Builder{}
			ml.ToMathMl(&sb, nil)
			assert.Equalf(t, tt.want, sb.String(), "MathMlFromAST(%v)", tt.input)
		})
	}
}
