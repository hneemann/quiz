package data

import (
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/parser2/funcGen"
	"github.com/hneemann/parser2/value"
	"io"
	"math"
	"strconv"
	"strings"
)

type InputType int

const (
	Checkbox InputType = iota
	Text
	Number
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
	lid       int
	cid       int
	tid       int
	Name      string
	Question  string
	Input     []Input
	Validator Validator
}

func (t *Task) LID() int {
	return t.lid
}

func (t *Task) CID() int {
	return t.cid
}
func (t *Task) TID() int {
	return t.tid
}

type Chapter struct {
	lid         int
	cid         int
	Name        string
	Description string
	Task        []*Task
}

func (c *Chapter) LID() int {
	return c.lid
}

func (c *Chapter) CID() int {
	return c.cid
}

type Lecture struct {
	lid         int
	Name        string
	Description string
	Chapter     []*Chapter
	files       map[string][]byte
}

func (l *Lecture) LID() int {
	return l.lid
}

func (l *Lecture) GetFile(name string) ([]byte, error) {
	if l.files == nil {
		return nil, fmt.Errorf("file '%s' not found", name)
	}
	if f, ok := l.files[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("file '%s' not found", name)
}

func (l *Lecture) Init() error {
	for cid, chapter := range l.Chapter {
		chapter.cid = cid
		for tid, task := range chapter.Task {
			task.cid = cid
			task.tid = tid

			m := make(map[string]struct{})
			for _, i := range task.Input {
				if _, ok := m[i.Id]; ok {
					return fmt.Errorf("duplicate input id %s in chapter %s task %s", i.Id, chapter.Name, task.Name)
				}
				m[i.Id] = struct{}{}

				err := i.Validator.test()
				if err != nil {
					return fmt.Errorf("test failed in input id %s in chapter %s task %s: %w", i.Id, chapter.Name, task.Name, err)
				}
			}

		}
	}
	return nil
}

type Lectures []*Lecture

func (l Lectures) Init() error {
	for lid, lecture := range l {
		lecture.lid = lid
		for _, chapter := range lecture.Chapter {
			chapter.lid = lid
			for _, task := range chapter.Task {
				task.lid = lid
			}
		}
	}
	return nil
}

func New(r io.Reader) (*Lecture, error) {
	var l Lecture
	err := xml.NewDecoder(r).Decode(&l)
	if err != nil {
		return nil, err
	}
	err = l.Init()
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

func cleanupError(err error) string {
	var nf parser2.NotFoundError
	if errors.As(err, &nf) {
		if len(nf.Avail()) > 0 {
			return fmt.Sprintf("'%s' kann nicht verwendet werden! Verfügbare Variablen sind: %s", nf.NotFound(), strings.Join(nf.Avail(), ", "))
		}
		return fmt.Sprintf("'%s' kann nicht verwendet werden!", nf.NotFound())
	}

	var gui GuiError
	if errors.As(err, &gui) {
		return gui.message
	}

	return "Es ist ein Fehler aufgetreten!"
}

func (v *Validator) Validate(m value.Map) (bool, string) {
	if v.Expression == "" {
		return true, ""
	}
	f, err := myParser.Generate(v.Expression, "a")
	if err != nil {
		return false, cleanupError(err)
	}
	r, err := f.Eval(m)
	if err != nil {
		return false, cleanupError(err)
	}
	switch r := r.(type) {
	case value.Bool:
		if r {
			return true, ""
		} else {
			if v.Help == "" {
				return false, DefaultMessage
			}
			return false, DefaultMessage + "\n\nHinweis: " + v.Help
		}
	case value.String:
		return false, string(r)
	default:
		return false, "unexpected result"
	}
}

const DefaultMessage = "Das ist nicht richtig!"

func (v *Validator) ToResultMap(m value.Map, id string, result map[string]string, showResult bool) {
	if ok, msg := v.Validate(m); !ok {
		if showResult {
			if v.Explanation != "" {
				if msg != "" {
					msg += "\n\n"
				}
				msg += "Lösung:\n\n" + v.Explanation
			}
		}
		result[id] = msg
	}
}

func (v *Validator) test() error {
	return nil
}

func (t *Task) Validate(input DataMap, showResult bool) map[string]string {
	m := value.NewMap(input)
	result := make(map[string]string)
	t.Validator.ToResultMap(m, "_task_", result, showResult)
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

type countVarUsageVisitor struct {
	n int
}

func (c *countVarUsageVisitor) Visit(ast parser2.AST) bool {
	switch a := ast.(type) {
	case *parser2.FunctionCall:
		for _, arg := range a.Args {
			arg.Traverse(c)
		}
		return false
	case *parser2.Const[value.Value]:
		c.n++
	case *parser2.Ident:
		c.n++
	}
	return true
}

type GuiError struct {
	message string
	cause   error
}

func (g GuiError) Error() string {
	return g.message
}

func (g GuiError) Unwrap() error {
	return g.cause
}

func createExpressionMethods(parser *parser2.Parser[value.Value]) value.MethodMap {
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
					return nil, GuiError{message: "Fehler im Ausdruck '" + e.expression + "'", cause: err}
				}
				return value.Float(r), nil
			} else {
				return nil, fmt.Errorf("expected a list, got %v", stack.Get(1))
			}
		}),
		"varUsages": value.MethodAtType(0, func(e Expression, stack funcGen.Stack[value.Value]) (value.Value, error) {
			ast, err := parser.Parse(e.expression)
			if err != nil {
				return nil, GuiError{message: "Fehler im Ausdruck '" + e.expression + "'", cause: err}
			}
			v := countVarUsageVisitor{}
			ast.Traverse(&v)
			return value.Int(v.n), nil
		}),
	}
}

const ExpressionTypeId = 10

func (e Expression) GetType() value.Type {
	return ExpressionTypeId
}

var myParser = value.New().
	AddFinalizerValue(func(f *value.FunctionGenerator) {

		f.AddStaticFunction("cmpFunc", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let soll=parseFunc(a,vars);
                                                        let ist=parseFunc(b,vars);
                                                        !values.present(x->abs(soll.eval(x)-ist.eval(x))>0.001)`, "a", "b", "vars", "values")),
			Args:   4,
			IsPure: true,
		}.SetDescription("func a", "func b", "argList", "values", "compares two functions"))
		f.AddStaticFunction("cmpFuncCplx", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let n=parseFunc(b,vars).varUsages();
                                                        let nMin=parseFunc(a,vars).varUsages();
                                                        n<=nMin`, "a", "b", "vars")),
			Args:   3,
			IsPure: true,
		}.SetDescription("func a", "func b", "argList", "compares complexity of two functions"))
		f.AddStaticFunction("cmpValues", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let isExp=parseFunc(isStr,[]);
                                                    let is=isExp.eval([]);
                                                    let dif=abs(is-expected)/expected*100;
                                                    dif<percent`, "expected", "isStr", "percent")),
			Args:   3,
			IsPure: true,
		}.SetDescription("expected", "is", "percent", "compares two values"))

		p := f.GetParser()
		f.RegisterMethods(ExpressionTypeId, createExpressionMethods(p))
		//p.SetNumberMatcher(number)
		p.TextOperator(map[string]string{"in": "~", "is": "=", "or": "|", "and": "&"})
	}).AddStaticFunction("parseFunc",
	funcGen.Function[value.Value]{
		Func: func(stack funcGen.Stack[value.Value], closureStore []value.Value) (value.Value, error) {
			if exp, ok := stack.Get(0).(value.String); ok {
				if exp == "" {
					return nil, GuiError{message: "Die Eingabe ist leer!"}
				}
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
	if len(expr) == 0 {
		return nil, fmt.Errorf("Der Ausdruck ist leer!")
	}
	fu, err := floatParser.Generate(expr, args...)
	if err != nil {
		return nil, GuiError{message: fmt.Sprintf("Der Ausdruck '%s' enthält Fehler!", expr), cause: err}
	}
	return Expression{expression: expr, fu: fu}, nil
}

var floatParser = funcGen.New[float64]().
	AddConstant("pi", math.Pi).
	AddConstant("e", math.E).
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
