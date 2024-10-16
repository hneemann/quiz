package data

import (
	"encoding/xml"
	"github.com/hneemann/parser2/value"
	"github.com/hneemann/parser2/value/export"
	"io"
	"strings"
)

type InputType int

const (
	Text InputType = iota
	Number
	Checkbox
)

func (it *InputType) UnmarshalText(text []byte) error {
	switch strings.ToLower(string(text)) {
	case "number":
		*it = Number
	case "checkbox":
		*it = Checkbox
	default:
		*it = Text
	}
	return nil
}

func (it InputType) MarshalText() ([]byte, error) {
	var name string
	switch it {
	case Number:
		name = "number"
	case Checkbox:
		name = "checkbox"
	default:
		name = "text"
	}
	return []byte(name), nil
}

type Validator struct {
	Expression  string
	Help        string
	Explanation string
}

type Input struct {
	Id        string `xml:"id,attr"`
	Label     string
	Type      InputType `xml:"type,attr"`
	Validator Validator
}

type Task struct {
	Question  string
	Input     []Input
	Validator Validator
}

type Chapter struct {
	Name        string
	Description string
	Task        []Task
}

type Lecture struct {
	Name        string
	Description string
	Chapter     []Chapter
}

func New(r io.Reader) (*Lecture, error) {
	var l Lecture
	err := xml.NewDecoder(r).Decode(&l)
	if err != nil {
		return nil, err
	}
	return &l, nil
}

type DataMap map[string]interface{}

func (d DataMap) Get(key string) (value.Value, bool) {
	v, ok := d[key]
	if !ok {
		return nil, false
	}
	return toValue(v)
}

func toValue(v interface{}) (value.Value, bool) {
	switch v := v.(type) {
	case string:
		return value.String(v), true
	case int:
		return value.Int(v), true
	case float64:
		return value.Float(v), true
	case bool:
		return value.Bool(v), true
	}
	return nil, false
}

func (d DataMap) Iter(yield func(key string, v value.Value) bool) bool {
	for k, v := range d {
		if v, ok := toValue(v); ok {
			if !yield(k, v) {
				return false
			}
		}
	}
	return true
}

func (d DataMap) Size() int {
	return len(d)
}

func (v *Validator) Validate(m value.Map, showResult bool) (bool, string) {
	f, err := myParser.GenerateWithMap(v.Expression, "this")
	if err != nil {
		return false, err.Error()
	}
	r, err := f.Eval(m)
	if err != nil {
		return false, err.Error()
	}
	switch r := r.(type) {
	case value.Bool:
		if r {
			return true, ""
		} else {
			if showResult {
				return false, v.Explanation
			}
			return false, v.Help
		}
	case value.String:
		return false, string(r)
	default:
		return false, "unexpected result"
	}
}

const DefaultMessage = "Das ist nicht richtig!"

func (v *Validator) ToResultMap(m value.Map, id string, result map[string]string, final bool) {
	if ok, msg := v.Validate(m, final); !ok {
		if msg == "" {
			msg = DefaultMessage
		}
		result[id] = msg
	}
}

func (t *Task) Validate(input DataMap, showResult bool) map[string]string {
	m := value.NewMap(input)
	result := make(map[string]string)
	t.Validator.ToResultMap(m, "Task", result, showResult)
	for _, i := range t.Input {
		i.Validator.ToResultMap(m, i.Id, result, showResult)
	}
	return result
}

var myParser = value.New().
	AddFinalizerValue(func(f *value.FunctionGenerator) {
		export.AddHTMLStylingHelpers(f)
		p := f.GetParser()
		//p.SetNumberMatcher(number)
		p.TextOperator(map[string]string{"in": "~", "is": "=", "or": "|", "and": "&"})
	})
