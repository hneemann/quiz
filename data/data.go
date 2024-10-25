package data

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/parser2/funcGen"
	"github.com/hneemann/parser2/value"
	"hash"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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

type Test struct {
	data   map[string]string
	result string
}

func (t *Test) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	data := make(map[string]string)
	for _, a := range start.Attr {
		data[a.Name.Local] = a.Value
	}
	var result string
	err := d.DecodeElement(&result, &start)
	if err != nil {
		return err
	}
	t.data = data
	t.result = result
	return nil
}

func (t *Test) String() string {
	var b strings.Builder
	for k, v := range t.data {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(fmt.Sprintf("%s=\"%v\"", k, v))
	}
	return b.String()
}

func (t *Test) test(fu funcGen.Func[value.Value], avail map[string]InputType) error {
	m := DataMap{}
	var expectedOkStr string
	for k, v := range t.data {
		if k != "ok" {
			if ty, ok := avail[k]; ok {
				switch ty {
				case Number, Text:
					m[k] = v
				case Checkbox:
					switch v {
					case "yes", "true":
						m[k] = true
					case "no", "false":
						m[k] = false
					default:
						return fmt.Errorf("attribute '%s' needs to be 'yes', 'no', 'true' or 'false', not '%s'", k, v)
					}
				}
			} else {
				return fmt.Errorf("unknown variable '%s'", k)
			}
		} else {
			expectedOkStr = v
		}

	}
	v, err := fu.Eval(value.NewMap(m))
	if err != nil {
		return err
	}

	if expectedOkStr != "" {
		expectedOk := false
		if expectedOkStr == "yes" {
			expectedOk = true
		} else if expectedOkStr != "no" {
			return fmt.Errorf("attribute 'ok' needs to be yes or no, not '%s'", expectedOkStr)
		}

		if isOk, ok := v.(value.Bool); ok {
			if bool(isOk) != expectedOk {
				return fmt.Errorf("expected %t, got %t", expectedOk, isOk)
			}
		} else {
			return fmt.Errorf("expected bool, got %T", v)
		}
	} else {
		if str, ok := v.(value.String); ok {
			if string(str) != t.result {
				return fmt.Errorf("expected '%s', got '%s'", t.result, str)
			}
		} else {
			return fmt.Errorf("expected string, got %T", v)
		}
	}

	return nil
}

type Validator struct {
	Expression  string
	Help        string
	Explanation string
	Test        []Test
	fu          funcGen.Func[value.Value]
}

type collectVars struct {
	used map[string]bool
}

func (c *collectVars) Visit(ast parser2.AST) bool {
	if a, ok := ast.(*parser2.MapAccess); ok {
		if i, ok := a.MapValue.(*parser2.Ident); ok {
			if i.Name == "a" {
				c.used[a.Key] = true
			}
		}
	}
	return true
}

// Init initializes the validator.
// If thisVar is not empty, it has to be a used in the expression.
// The vars map contains all variables that can be used in the expression.
func (v *Validator) init(varsAvail map[string]InputType, mustBeUsed []string) error {
	if strings.TrimSpace(v.Expression) == "" {
		return fmt.Errorf("no expression given")
	}

	f, err := myParser.Generate(v.Expression, "a")
	if err != nil {
		return err
	}
	v.fu = f

	a, err := myParser.GetParser().Parse(v.Expression)
	if err != nil {
		return err
	}

	varsUsed := collectVars{used: make(map[string]bool)}
	a.Traverse(&varsUsed)

	if len(varsUsed.used) == 0 {
		return fmt.Errorf("no variable is used")
	}

	for vu := range varsUsed.used {
		if _, ok := varsAvail[vu]; !ok {
			return fmt.Errorf("'%s' is used but not available", vu)
		}
	}

	for _, va := range mustBeUsed {
		if !varsUsed.used[va] {
			return fmt.Errorf("'%s' is not used in expression", va)
		}
	}

	for _, t := range v.Test {
		err = t.test(v.fu, varsAvail)
		if err != nil {
			return fmt.Errorf("error in test <test %s>: %w", t.String(), err)
		}
	}

	return nil
}

func cleanupError(err error) string {
	var notFound parser2.NotFoundError
	if errors.As(err, &notFound) {
		if len(notFound.Avail()) > 0 {
			return fmt.Sprintf("'%s' kann nicht verwendet werden! Verfügbare Variablen sind: %s", notFound.NotFound(), strings.Join(notFound.Avail(), ", "))
		}
		return fmt.Sprintf("'%s' kann nicht verwendet werden!", notFound.NotFound())
	}

	var notAFunc parser2.NotAFunction
	if errors.As(err, &notAFunc) {
		return fmt.Sprintf("Zwischen einer Variablen und einer öffnenden Klammer fehlt ein Leerzeichen: $%s$", notAFunc.NotFound())
	}

	var gui GuiError
	if errors.As(err, &gui) {
		return gui.message
	}

	log.Print("unexpected error:", err)
	return "Der eingegebene Ausdruck ist ungültig!"
}

const DefaultMessage = "Das ist nicht richtig!"

func (v *Validator) Validate(m value.Map) (bool, string) {
	if v == nil {
		return true, ""
	}

	r, err := v.fu.Eval(m)
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
		if v.Help == "" {
			return false, string(r)
		}
		return false, string(r) + "\n\nHinweis: " + v.Help
	default:
		return false, "unexpected result"
	}
}

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

type Input struct {
	Id        string `xml:"id,attr"`
	Label     string
	Type      InputType `xml:"type,attr"`
	Validator *Validator
}

type Task struct {
	lid       string
	lHash     string
	cid       int
	tid       int
	Name      string
	Question  string
	Input     []*Input
	Validator *Validator
}

type InnerId struct {
	CId int
	TId int
}

type TaskId struct {
	LHash   string
	InnerId InnerId
}

func (t *Task) LID() string {
	return t.lid
}

func (t *Task) CID() int {
	return t.cid
}
func (t *Task) TID() int {
	return t.tid
}

type Chapter struct {
	lid         string
	cid         int
	Title       string
	Description string
	Task        []*Task
}

func (c *Chapter) LID() string {
	return c.lid
}

func (c *Chapter) CID() int {
	return c.cid
}

func (c *Chapter) GetTask(number int) (*Task, error) {
	if number < 0 || number >= len(c.Task) {
		return nil, fmt.Errorf("task %d not found", number)
	}
	return c.Task[number], nil
}

type Lecture struct {
	Id          string `xml:"id,attr"`
	Title       string
	Author      string
	AuthorEMail string
	Description string
	hash        string
	Chapter     []*Chapter
	folder      string
	files       map[string][]byte
}

func (l *Lecture) Hash() string {
	return l.hash
}

func (l *Lecture) LID() string {
	return l.Id
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
	if l.Title == "" {
		return errors.New("lecture has no title")
	}
	if l.Author == "" {
		return fmt.Errorf("author is missing in lecture %s", l.Title)
	}
	if l.AuthorEMail == "" {
		return fmt.Errorf("author email is missing in lecture %s", l.Title)
	}

	for cid, chapter := range l.Chapter {
		if chapter.Title == "" {
			return fmt.Errorf("no title in chapter %d", cid)
		}
		chapter.cid = cid
		for tid, task := range chapter.Task {
			task.cid = cid
			task.tid = tid
			task.lHash = l.hash

			if task.Name == "" {
				task.Name = fmt.Sprintf("Aufgabe %d", tid+1)
			} else {
				task.Name = fmt.Sprintf("Aufgabe %d: %s", tid+1, task.Name)
			}

			vars := make(map[string]InputType)
			for _, i := range task.Input {
				if i.Id == "" {
					return fmt.Errorf("no id at input in chapter '%s' task '%s'", chapter.Title, task.Name)
				}

				if !isIdent(i.Id) {
					return fmt.Errorf("invalid id '%s' at input in chapter '%s' task '%s'", i.Id, chapter.Title, task.Name)
				}

				if _, ok := vars[i.Id]; ok {
					return fmt.Errorf("duplicate input id '%s' in chapter '%s' task '%s'", i.Id, chapter.Title, task.Name)
				}
				vars[i.Id] = i.Type

				if i.Label == "" {
					return fmt.Errorf("no label at input id '%s' in chapter '%s' task '%s'", i.Id, chapter.Title, task.Name)
				}
			}

			var needsToBeUsedInTaskValidator []string
			for _, i := range task.Input {
				if i.Validator != nil {
					err := i.Validator.init(vars, []string{i.Id})
					if err != nil {
						return fmt.Errorf("invalid expression in input id '%s' in chapter '%s' task '%s': %w", i.Id, chapter.Title, task.Name, err)
					}
				} else {
					needsToBeUsedInTaskValidator = append(needsToBeUsedInTaskValidator, i.Id)
				}

				if task.Validator == nil && i.Validator == nil {
					return fmt.Errorf("validator is missing in input id '%s' in chapter '%s' task '%s'", i.Id, chapter.Title, task.Name)
				}
			}

			if task.Validator != nil {
				err := task.Validator.init(vars, needsToBeUsedInTaskValidator)
				if err != nil {
					return fmt.Errorf("invalid expression in chapter '%s' task '%s': %w", chapter.Title, task.Name, err)
				}
			} else {
				if len(needsToBeUsedInTaskValidator) > 0 {
					return fmt.Errorf("validator is missing in chapter '%s' task '%s'", chapter.Title, task.Name)
				}
			}

		}
	}
	return nil
}

func isIdent(id string) bool {
	for i, c := range id {
		if i == 0 {
			if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				continue
			}
			return false
		} else {
			if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				continue
			}
			return false
		}
	}
	return true
}

func (l *Lecture) GetChapter(number int) (*Chapter, error) {
	if number < 0 || number >= len(l.Chapter) {
		return nil, fmt.Errorf("chapter %d not found", number)
	}
	return l.Chapter[number], nil
}

func (l *Lecture) CanReload() bool {
	return l.folder != ""
}

type Lectures struct {
	rwMutex  sync.RWMutex
	lectures map[string]*Lecture
	list     []*Lecture
	folder   string
}

func (l *Lectures) insert(lecture *Lecture) {
	l.rwMutex.Lock()
	defer l.rwMutex.Unlock()

	if l.lectures == nil {
		l.lectures = make(map[string]*Lecture)
	}

	l.lectures[lecture.Id] = lecture
	l.init()
}

func (l *Lectures) init() {
	lectureList := make([]*Lecture, 0, len(l.lectures))
	for lid, lecture := range l.lectures {
		lectureList = append(lectureList, lecture)
		for _, chapter := range lecture.Chapter {
			chapter.lid = lid
			for _, task := range chapter.Task {
				task.lid = lid
			}
		}
	}
	sort.Slice(lectureList, func(i, j int) bool {
		return lectureList[i].Title < lectureList[j].Title
	})
	l.list = lectureList
}

func (l *Lectures) List() []*Lecture {
	l.rwMutex.RLock()
	defer l.rwMutex.RUnlock()
	return l.list
}

func (l *Lectures) GetLecture(id string) (*Lecture, error) {
	l.rwMutex.RLock()
	defer l.rwMutex.RUnlock()

	if l.lectures != nil {
		if lecture, ok := l.lectures[id]; ok {
			return lecture, nil
		}
	}
	return nil, fmt.Errorf("lecture %s not found", id)
}

func (l *Lectures) add(lecture *Lecture) {
	if l.lectures == nil {
		l.lectures = make(map[string]*Lecture)
	}
	l.lectures[lecture.Id] = lecture
}

func (l *Lectures) Uploaded(file []byte) error {
	lecture, err := ReadZip(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(l.folder, lecture.Id+".zip"))
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(file)

	l.insert(lecture)

	return nil
}

func (l *Lectures) Reload(id string) (*Lecture, error) {
	if l.lectures == nil {
		return nil, fmt.Errorf("no lectures available")
	}

	l.rwMutex.Lock()
	defer l.rwMutex.Unlock()

	if lecture, ok := l.lectures[id]; !ok {
		return nil, fmt.Errorf("lecture %s not found", id)
	} else {
		newLecture, err := readFolder(lecture.folder)
		if err != nil {
			return nil, err
		}

		l.lectures[id] = newLecture
		l.init()
		return newLecture, nil
	}
}

type hashReader struct {
	parent io.Reader
	hasher hash.Hash
}

func (h *hashReader) Read(p []byte) (n int, err error) {
	n, err = h.parent.Read(p)
	if n > 0 {
		h.hasher.Write(p[:n])
	}
	return
}

func (h *hashReader) get() string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(h.hasher.Sum(nil))
}

func New(r io.Reader) (*Lecture, error) {
	var l Lecture
	h := hashReader{parent: r, hasher: sha1.New()}
	err := xml.NewDecoder(&h).Decode(&l)
	if err != nil {
		return nil, err
	}
	l.hash = h.get()

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

func (t *Task) Validate(input DataMap, showResult bool) map[string]string {
	m := value.NewMap(input)
	result := make(map[string]string)
	t.Validator.ToResultMap(m, "_task_", result, showResult)
	for _, i := range t.Input {
		i.Validator.ToResultMap(m, i.Id, result, showResult)
	}

	return result
}

func (t *Task) GetId() TaskId {
	return TaskId{LHash: t.lHash, InnerId: InnerId{CId: t.cid, TId: t.tid}}
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

func (e Expression) ToString(funcGen.Stack[value.Value]) (string, error) {
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
						args[i] = f
					} else {
						return nil, fmt.Errorf("expected float, got %v", v)
					}
				}
				r, err := e.fu.Eval(args...)
				if err != nil {
					return nil, GuiError{message: "Fehler bei der Berechnung von '" + e.expression + "'", cause: err}
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
                                                        !values.present(x->abs(soll.eval(x)-ist.eval(x))>0.0001)`, "a", "b", "vars", "values")),
			Args:   4,
			IsPure: true,
		}.SetDescription("func a", "func b", "argList", "values",
			"compares two functions by evaluating them for a list of arguments.\n"+
				"It returns true if the difference between the two functions is less than 0.0001 for all arguments"))
		f.AddStaticFunction("cmpFuncCplx", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let n=parseFunc(b,vars).varUsages();
                                                        let nMin=parseFunc(a,vars).varUsages();
                                                        n<=nMin`, "a", "b", "vars")),
			Args:   3,
			IsPure: true,
		}.SetDescription("func a", "func b", "argList",
			"compares complexity of two functions. It returns true if the complexity of the second function\n"+
				"is less or equal to the complexity of the first function"))
		f.AddStaticFunction("cmpValues", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let isExp=parseFunc(isStr,[]);
                                                    let is=isExp.eval([]);
													if expected=0 
                                                    then abs(is)<percent/100
                                                    else
                                                      let dif=abs((is-expected)/expected*100);
                                                      dif<percent`, "expected", "isStr", "percent")),
			Args:   3,
			IsPure: true,
		}.SetDescription("expected", "is", "percent",
			"compares two values and returns true if the difference is less than the given percent of the expected value"))

		p := f.GetParser()
		f.RegisterMethods(ExpressionTypeId, createExpressionMethods(p))
		//p.SetNumberMatcher(number)
		p.TextOperator(map[string]string{"in": "~", "is": "=", "or": "|", "and": "&"})
	}).
	AddStaticFunction("out", funcGen.Function[value.Value]{
		Func: func(stack funcGen.Stack[value.Value], closureStore []value.Value) (value.Value, error) {
			v := stack.Get(0)
			log.Print(v)
			return v, nil
		},
		Args:   1,
		IsPure: true,
	}.SetDescription("val", "writes a value to the log and returns the value.")).
	AddStaticFunction("parseFunc",
		funcGen.Function[value.Value]{
			Func: func(stack funcGen.Stack[value.Value], closureStore []value.Value) (value.Value, error) {
				if exp, ok := stack.Get(0).(value.String); ok {
					if exp == "" {
						return nil, GuiError{message: "Die Eingabe ist leer!"}
					}
					if list, ok := stack.Get(1).(*value.List); ok {
						var args []string
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
		log.Print("error parsing expression:", err)
		return nil, GuiError{message: fmt.Sprintf("Der Ausdruck '%s' enthält Fehler und kann nicht analysiert werden!", expr), cause: err}
	}
	return Expression{expression: expr, fu: fu}, nil
}

var floatParser = funcGen.New[float64]().
	SetComfort(true).
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
