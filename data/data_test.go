package data

import (
	"github.com/hneemann/parser2/value"
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestSimple(t *testing.T) {
	xml := `<Lecture id="EL1">
    <Title>Elektronik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Description>In diesem Quiz werden Fragen zur Elektronik 1 gestellt.</Description>
    <Chapter>
        <Title>Die Diode</Title>
        <Description>Hier geht es um die Eigenschaften der Diode.</Description>
        <Task>
            <Name>Allgemein</Name>
            <Question>Eine Diode ...</Question>
            <Input id="elBau" type="checkbox">
                <Label>ist ein elektronisches Bauteil.</Label>
            </Input>
            <Input id="zwei" type="checkbox">
                <Label>hat zwei Anschlüsse</Label>
            </Input>
            <Input id="drei" type="checkbox">
                <Label>hat drei Anschlüsse</Label>
            </Input>
            <Input id="linear" type="checkbox">
                <Label>zeigt lineares Verhalten.</Label>
            </Input>
            <Input id="nlinear" type="checkbox">
                <Label>zeigt nicht lineares Verhalten.</Label>
            </Input>
            <Validator>
                <Expression>
                    answer.elBau and answer.zwei and !answer.drei and !answer.linear and answer.nlinear
                </Expression>
                <Explanation>Die Diode ist ein nichtlineares elektronisches Bauteil mit zwei Anschlüssen.</Explanation>
            </Validator>
        </Task>
	</Chapter>
</Lecture>
`
	lecture, err := New(strings.NewReader(xml))
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
}

func TestSimple2(t *testing.T) {
	xml := `<Lecture id="EL1">
    <Title>Elektronik 1</Title>
    <Author>Prof. Dr. Helmut Neemann</Author>
    <AuthorEMail>helmut.neemann@dhbw.de</AuthorEMail>
    <Description>In diesem Quiz werden Fragen zur Elektronik 1 gestellt.</Description>
    <Chapter>
        <Title>Die Diode</Title>
        <Description>Hier geht es um die Eigenschaften der Diode.</Description>
        <Task>
            <Name>Näherung im Durchlassbereich</Name>
            <Question>
Die Diodengleichung lautet $I_D=I_S (e^{\frac{U_D}{U_T}}-1)$.

Wie könnte eine gute Näherung für die Gleichung im Durchlassbereich ($U_D>0.5\u{V}$) aussehen?</Question>
            <Input id="func1" type="text">
                <Label>$I_D=$</Label>
                <Validator>
                    <Expression>cmpFunc("is*exp(ud/ut)",answer.func1.toLower(),["is","ud","ut"],[[1,1,2],[0.001,1,3],[2,1,4]])</Expression>
                    <Help>Überlegen Sie sich, welchen Summanden man vernachlässigen kann.</Help>
                    <Explanation>Die Näherung lautet $I_D=I_S e^{\frac{U_D}{U_T}}$.</Explanation>
                </Validator>
            </Input>
        </Task>
	</Chapter>
</Lecture>
`
	lecture, err := New(strings.NewReader(xml))
	assert.NoError(t, err)

	task2 := lecture.Chapter[0].Task[0]

	input := make(DataMap)
	input["func1"] = "IS*exp(UD/UT)"
	result := task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func1"] = "exp(UD/UT)*IS"
	result = task2.Validate(input, false)
	assert.Equal(t, 0, len(result))

	input = make(DataMap)
	input["func1"] = "3*x^2+1"
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

func TestValidator(t *testing.T) {
	test := []struct {
		expr    string
		inputs  map[InputId]InputType
		used    []InputId
		isValid bool
	}{
		{"1+2", map[InputId]InputType{"x": Text}, nil, false},
		{"1+answer.x", map[InputId]InputType{"x": Text}, nil, true},
		{"1+answer.y", map[InputId]InputType{"x": Text}, nil, false},
		{"1+2", map[InputId]InputType{"x": Text}, []InputId{"x"}, false},
		{"1+answer.x", map[InputId]InputType{"x": Text}, []InputId{"x"}, true},
		{"1+answer.y", map[InputId]InputType{"x": Text}, []InputId{"x"}, false},
		{"1+2", map[InputId]InputType{"x": Text, "y": Text}, nil, false},
		{"answer.x+answer.y", map[InputId]InputType{"x": Text, "y": Text}, nil, true},
	}

	for _, tst := range test {
		t.Run(tst.expr, func(t *testing.T) {
			val := Validator{Expression: tst.expr}
			err := val.init(tst.inputs, tst.used)
			if tst.isValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func Test_cleanUpMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"noNewLine", "a", "a"},
		{"nl at end", "a\nb\n", "a\nb\n"},
		{"space at eol", "a \nb \n ", "a\nb\n"},
		{"simple1", "a\nb\nc", "a\nb\nc"},
		{"simple2", "  a\n  b\n  c", "a\nb\nc"},
		{"simple3", "  a\n   b\n  c", "a\n b\nc"},
		{"tab", "\ta\n    b\n\tc", "a\nb\nc"},
		{"emptyLine1", "  a\n   \t\n  c", "a\n\nc"},
		{"emptyLine1", "  a\n   \t\n   c", "a\n\n c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, cleanUpMarkdown(tt.in), "cleanUpMarkdown(%v)", tt.in)
		})
	}
}
