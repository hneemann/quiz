package data

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/parser2/funcGen"
	"github.com/hneemann/parser2/value"
	"github.com/hneemann/parser2/value/export"
	"io"
	"math"
	"strconv"
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
	if v.Expression == "" {
		return true, ""
	}
	f, err := myParser.Generate(v.Expression, "a")
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

type Expression struct {
	expression string
	fu         funcGen.Func[float64]
}

func (e Expression) ToList() (*value.List, bool) {
	return nil, false
}

func (e Expression) ToMap() (value.Map, bool) {
	return value.Map{}, false
}

func (e Expression) ToInt() (int, bool) {
	return 0, false
}

func (e Expression) ToFloat() (float64, bool) {
	return 0, false
}

func (e Expression) ToString(st funcGen.Stack[value.Value]) (string, error) {
	return e.expression, nil
}

func (e Expression) ToBool() (bool, bool) {
	return false, false
}

func (e Expression) ToClosure() (funcGen.Function[value.Value], bool) {
	return funcGen.Function[value.Value]{}, false
}

func createExpressionMethods() value.MethodMap {
	return value.MethodMap{
		"eval": value.MethodAtType(1, func(e Expression, stack funcGen.Stack[value.Value]) (value.Value, error) {
			if argList, ok := stack.Get(1).(*value.List); ok {
				argValues, err := argList.ToSlice(stack)
				if err != nil {
					return nil, err
				}
				args := make([]float64, len(argValues))
				for i, v := range argValues {
					if f, ok := v.ToFloat(); ok {
						args[i] = float64(f)
					} else {
						return nil, fmt.Errorf("expected float, got %v", v)
					}
				}
				r, err := e.fu.Eval(args...)
				if err != nil {
					var er parser2.NotFoundError
					if errors.As(err, &er) {
						return nil, fmt.Errorf("'%s' nicht erwartet in '%s'", er.NotFound(), e.expression)
					}
					return nil, fmt.Errorf("Der Ausdruck '%s' konnte nicht berechnet werden!", e.expression)
				}
				return value.Float(r), nil
			} else {
				return nil, fmt.Errorf("expected a list, got %v", stack.Get(1))
			}
		}),
	}
}

const ExpressionTypeId = 10

func (e Expression) GetType() value.Type {
	return ExpressionTypeId
}

var myParser = value.New().
	AddFinalizerValue(func(f *value.FunctionGenerator) {
		export.AddHTMLStylingHelpers(f)
		p := f.GetParser()
		f.RegisterMethods(ExpressionTypeId, createExpressionMethods())
		//p.SetNumberMatcher(number)
		p.TextOperator(map[string]string{"in": "~", "is": "=", "or": "|", "and": "&"})
	}).AddStaticFunction("parseFunc",
	funcGen.Function[value.Value]{
		Func: func(stack funcGen.Stack[value.Value], closureStore []value.Value) (value.Value, error) {
			if exp, ok := stack.Get(0).(value.String); ok {
				if list, ok := stack.Get(1).(*value.List); ok {
					args := []string{}
					argValues, err := list.ToSlice(stack)
					if err != nil {
						return nil, err
					}
					for _, v := range argValues {
						if str, ok := v.(value.String); ok {
							args = append(args, string(str))
						} else {
							return nil, fmt.Errorf("expected string, got %v", v)
						}
					}
					return createExpression(string(exp), args)
				} else {
					return nil, fmt.Errorf("expected a list, got %v", stack.Get(1))
				}
			} else {
				return nil, fmt.Errorf("expected string, got %v", stack.Get(0))
			}
		},
		Args:   2,
		IsPure: true,
	}.SetDescription("strFunc", "listOfArgs", "parse a function using the list of arguments"))

func createExpression(expr string, args []string) (value.Value, error) {
	fu, err := floatParser.Generate(expr, args...)
	if err != nil {
		var e parser2.NotFoundError
		if errors.As(err, &e) {
			return nil, fmt.Errorf("'%s' nicht erwartet in '%s'", e.NotFound(), expr)
		}
		return nil, fmt.Errorf("Ungültiger Ausdruck '%s'", expr)
	}
	return Expression{expression: expr, fu: fu}, nil
}

var floatParser = funcGen.New[float64]().
	AddConstant("pi", math.Pi).
	AddSimpleOp("=", true, func(a, b float64) (float64, error) { return fromBool(a == b), nil }).
	AddSimpleOp("<", false, func(a, b float64) (float64, error) { return fromBool(a < b), nil }).
	AddSimpleOp(">", false, func(a, b float64) (float64, error) { return fromBool(a > b), nil }).
	AddSimpleOp("+", true, func(a, b float64) (float64, error) { return a + b, nil }).
	AddSimpleOp("-", false, func(a, b float64) (float64, error) { return a - b, nil }).
	AddSimpleOp("*", true, func(a, b float64) (float64, error) { return a * b, nil }).
	AddSimpleOp("/", false, func(a, b float64) (float64, error) { return a / b, nil }).
	AddSimpleOp("^", false, func(a, b float64) (float64, error) { return math.Pow(a, b), nil }).
	AddUnary("-", func(a float64) (float64, error) { return -a, nil }).
	AddSimpleFunction("sin", math.Sin).
	AddSimpleFunction("cos", math.Cos).
	AddSimpleFunction("tan", math.Tan).
	AddSimpleFunction("exp", math.Exp).
	AddSimpleFunction("ln", math.Log).
	AddSimpleFunction("sqrt", math.Sqrt).
	AddSimpleFunction("sqr", func(x float64) float64 {
		return x * x
	}).
	SetToBool(func(c float64) (bool, bool) { return c != 0, true }).
	SetNumberParser(
		parser2.NumberParserFunc[float64](
			func(n string) (float64, error) {
				return strconv.ParseFloat(n, 64)
			},
		),
	)

func fromBool(b bool) float64 {
	if b {
		return 1
	} else {
		return 0
	}
}
