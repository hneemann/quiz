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
		{input: "(a+b)/(b+1)", want: "<mfrac><mrow><mi>a</mi><mo>+</mo><mi>b</mi></mrow><mrow><mi>b</mi><mo>+</mo><mn>1</mn></mrow></mfrac>"},
		{input: "sin(t)", want: "<mrow><mi>sin</mi><mo>(</mo><mi>t</mi><mo>)</mo></mrow>"},
		{input: "atan2(y,x)", want: "<mrow><mi>atan2</mi><mo>(</mo><mrow><mi>y</mi><mo>,</mo><mi>x</mi></mrow><mo>)</mo></mrow>"},
		{input: "sqrt(x)", want: "<msqrt><mi>x</mi></msqrt>"},
		{input: "a^2", want: "<msup><mi>a</mi><mn>2</mn></msup>"},
		{input: "(a+1)^(i+2)", want: "<msup><mrow><mo>(</mo><mrow><mi>a</mi><mo>+</mo><mn>1</mn></mrow><mo>)</mo></mrow><mrow><mi>i</mi><mo>+</mo><mn>2</mn></mrow></msup>"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			a, err := floatParser.GetParser().Parse(tt.input)
			assert.NoError(t, err)
			ml, err := MathMlFromAST(a)
			assert.NoError(t, err)
			sb := strings.Builder{}
			ml.ToMathMl(&sb, nil)
			assert.Equalf(t, tt.want, sb.String(), "MathMlFromAST(%v)", tt.input)
		})
	}
}
