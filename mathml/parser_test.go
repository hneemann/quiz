package mathml

import (
	"strings"
	"testing"
)
import "github.com/stretchr/testify/assert"

func TestParse(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"1", "<mn>1</mn>"},
		{"1+2", "<mrow><mn>1</mn><mo>+</mo><mn>2</mn></mrow>"},
		{"1?2", "<mrow><mn>1</mn><mo>?</mo><mn>2</mn></mrow>"},
		{"1 + 2", "<mrow><mn>1</mn><mo>+</mo><mn>2</mn></mrow>"},
		{"(1 + 2)", "<mrow><mo>(</mo><mrow><mn>1</mn><mo>+</mo><mn>2</mn></mrow><mo>)</mo></mrow>"},
		{"x^2", "<msup><mi>x</mi><mn>2</mn></msup>"},
		{"(x+1)^2", "<msup><mrow><mo>(</mo><mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow><mo>)</mo></mrow><mn>2</mn></msup>"},
		{"x^{n+2}", "<msup><mi>x</mi><mrow><mi>n</mi><mo>+</mo><mn>2</mn></mrow></msup>"},
		{"\\frac{x+1}{x^2+1}", "<mfrac><mrow><mi>x</mi><mo>+</mo><mn>1</mn></mrow><mrow><msup><mi>x</mi><mn>2</mn></msup><mo>+</mo><mn>1</mn></mrow></mfrac>"},
		{"\\sqrt{x}", "<msqrt><mi>x</mi></msqrt>"},
		{"sin(\\omega t+\\phi)", "<mrow><mi>sin</mi><mrow><mo>(</mo><mrow><mi>&omega;</mi><mi>t</mi><mo>+</mo><mi>&phi;</mi></mrow><mo>)</mo></mrow></mrow>"},
		{"\\frac{1}{2}x^{\\frac{1}{3}}", "<mrow><mfrac><mn>1</mn><mn>2</mn></mfrac><msup><mi>x</mi><mfrac><mn>1</mn><mn>3</mn></mfrac></msup></mrow>"},
		{"x_{1,2}=\\frac{-b\\pm\\sqrt{b^2-4a c}}{2a}", "<mrow><msub><mi>x</mi><mrow><mn>1</mn><mo>,</mo><mn>2</mn></mrow></msub><mo>=</mo><mfrac><mrow><mo>-</mo><mi>b</mi><mo>&PlusMinus;</mo><msqrt><mrow><msup><mi>b</mi><mn>2</mn></msup><mo>-</mo><mn>4</mn><mi>a</mi><mi>c</mi></mrow></msqrt></mrow><mrow><mn>2</mn><mi>a</mi></mrow></mfrac></mrow>"},
		{"x_{1,2}=-\\frac{p}{2}\\pm\\sqrt{\\frac{p^2}{4}-q}", "<mrow><msub><mi>x</mi><mrow><mn>1</mn><mo>,</mo><mn>2</mn></mrow></msub><mo>=</mo><mo>-</mo><mfrac><mi>p</mi><mn>2</mn></mfrac><mo>&PlusMinus;</mo><msqrt><mrow><mfrac><msup><mi>p</mi><mn>2</mn></msup><mn>4</mn></mfrac><mo>-</mo><mi>q</mi></mrow></msqrt></mrow>"},
		{"e^{i\\gamma}=\\cos(\\gamma)+i \\sin(\\gamma)", "<mrow><msup><mi>e</mi><mrow><mi>i</mi><mi>&gamma;</mi></mrow></msup><mo>=</mo><mi>cos</mi><mrow><mo>(</mo><mi>&gamma;</mi><mo>)</mo></mrow><mo>+</mo><mi>i</mi><mi>sin</mi><mrow><mo>(</mo><mi>&gamma;</mi><mo>)</mo></mrow></mrow>"},
		{"x_n^2", "<msubsup><mi>x</mi><mi>n</mi><mn>2</mn></msubsup>"},
		{"x^2_n", "<msubsup><mi>x</mi><mi>n</mi><mn>2</mn></msubsup>"},
		{"\\int\\frac{1}{x^2} \\dif x=-\\frac{1}{x}", "<mrow><mo>&int;</mo><mfrac><mn>1</mn><msup><mi>x</mi><mn>2</mn></msup></mfrac><mn>d</mn><mi>x</mi><mo>=</mo><mo>-</mo><mfrac><mn>1</mn><mi>x</mi></mfrac></mrow>"},
		{"\\oint_S\\vec{H}\\cdot\\vec{\\dif s}=\\Theta", "<mrow><munder><mo>&oint;</mo><mi>S</mi></munder><mover><mi>H</mi><mo>&rarr;</mo></mover><mo>&middot;</mo><mover><mrow><mn>d</mn><mi>s</mi></mrow><mo>&rarr;</mo></mover><mo>=</mo><mi>&Theta;</mi></mrow>"},
		{"\\chi\\mu\\epsilon", "<mrow><mi>&chi;</mi><mi>&mu;</mi><mi>&epsilon;</mi></mrow>"},
		{"f(x_0)\\overset{!}{=}0", "<mrow><mi>f</mi><mrow><mo>(</mo><msub><mi>x</mi><mn>0</mn></msub><mo>)</mo></mrow><mover><mo>=</mo><mo>!</mo></mover><mn>0</mn></mrow>"},
		{"\\underset{n\\rightarrow\\infty}{\\lim}\\frac{1}{n}=0", "<mrow><munder><mi>lim</mi><mrow><mi>n</mi><mo>&rightarrow;</mo><mn>&infin;</mn></mrow></munder><mfrac><mn>1</mn><mi>n</mi></mfrac><mo>=</mo><mn>0</mn></mrow>"},
		{"\\sin(x)=\\sum_{k=0}^{\\infty}(-1)^k\\frac{x^{2k+1}}{(2k+1)!}", "<mrow><mi>sin</mi><mrow><mo>(</mo><mi>x</mi><mo>)</mo></mrow><mo>=</mo><munderover><mo>&sum;</mo><mrow><mi>k</mi><mo>=</mo><mn>0</mn></mrow><mn>&infin;</mn></munderover><msup><mrow><mo>(</mo><mrow><mo>-</mo><mn>1</mn></mrow><mo>)</mo></mrow><mi>k</mi></msup><mfrac><msup><mi>x</mi><mrow><mn>2</mn><mi>k</mi><mo>+</mo><mn>1</mn></mrow></msup><mrow><mrow><mo>(</mo><mrow><mn>2</mn><mi>k</mi><mo>+</mo><mn>1</mn></mrow><mo>)</mo></mrow><mo>!</mo></mrow></mfrac></mrow>"},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			s, err := LaTeXtoMathMLString(test.in)
			assert.NoError(t, err)
			assert.Equal(t, test.out, s)
		})
	}
}

func TestParseError(t *testing.T) {
	tests := []struct {
		in     string
		errMes string
	}{
		{"x^", "expected {, got EOF()"},
		{"x^{{}", "unexpected token: OpenBrace({)"},
	}

	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			_, err := ParseLaTeX(test.in)
			assert.Error(t, err)
			con := strings.Contains(err.Error(), test.errMes)
			if !con {
				t.Log(err)
			}
			assert.True(t, con)
		})
	}
}

func TestScanDollar(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "Empty",
			text: "Hello $$ World",
			want: "Hello $ World",
		},
		{
			name: "Simple",
			text: "Hello $x^2$ World",
			want: "Hello <math xmlns=\"&mathml;\"><msup><mi>x</mi><mn>2</mn></msup></math> World",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, ScanDollar(tt.text), "ScanDollar(%v)", tt.text)
		})
	}
}
