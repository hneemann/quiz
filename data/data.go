package data

import (
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/hneemann/parser2"
	"github.com/hneemann/parser2/funcGen"
	"github.com/hneemann/parser2/value"
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
	data   map[InputId]string
	result string
}

func (t *Test) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	data := make(map[InputId]string)
	for _, a := range start.Attr {
		data[InputId(a.Name.Local)] = a.Value
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

func (t *Test) test(fu funcGen.Func[value.Value], avail map[InputId]InputType) error {
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
	used map[InputId]bool
}

func (c *collectVars) Visit(ast parser2.AST) bool {
	if a, ok := ast.(*parser2.MapAccess); ok {
		if i, ok := a.MapValue.(*parser2.Ident); ok {
			if i.Name == "answer" {
				c.used[InputId(a.Key)] = true
			}
		}
	}
	return true
}

// Init initializes the validator.
// If thisVar is not empty, it has to be a used in the expression.
// The vars map contains all variables that can be used in the expression.
func (v *Validator) init(varsAvail map[InputId]InputType, mustBeUsed []InputId) error {
	if strings.TrimSpace(v.Expression) == "" {
		return fmt.Errorf("no expression given")
	}

	v.Help = cleanUpMarkdown(v.Help)

	f, err := myParser.Generate(v.Expression, "answer")
	if err != nil {
		return err
	}
	v.fu = f

	a, err := myParser.GetParser().Parse(v.Expression)
	if err != nil {
		return err
	}

	varsUsed := collectVars{used: make(map[InputId]bool)}
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

func (v *Validator) ToResultMap(m value.Map, id InputId, result map[InputId]string, showResult bool) {
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

type (
	TaskId     string
	InputId    string
	LectureId  string
	ChapterNum []int
	TaskNum    int
)

func (c ChapterNum) String() string {
	var b strings.Builder
	for i, n := range c {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}

func NewChapterNum(s string) (ChapterNum, error) {
	var c ChapterNum
	for _, n := range strings.Split(s, ".") {
		i, err := strconv.Atoi(n)
		if err != nil {
			return nil, err
		}
		c = append(c, i)
	}
	return c, nil
}

type Input struct {
	Id        InputId `xml:"id,attr"`
	Label     string
	Type      InputType `xml:"type,attr"`
	Validator *Validator
}

type Task struct {
	chapter           *Chapter
	num               TaskNum
	tid               TaskId
	inputHasValidator map[InputId]bool
	Name              string
	Question          string
	Input             []*Input
	Validator         *Validator
}

func (t *Task) Chapter() *Chapter {
	return t.chapter
}

func (t *Task) Num() TaskNum {
	return t.num
}

func (t *Task) TID() TaskId {
	return t.tid
}

func (t *Task) InputHasValidator(id InputId) bool {
	if has, ok := t.inputHasValidator[id]; ok {
		return has
	}
	return false
}

type Chapter struct {
	Include       string `xml:"file,attr"`
	lecture       *Lecture
	num           ChapterNum
	StepByStep    bool `xml:"stepByStep,attr"`
	Title         string
	Description   string
	Task          []*Task
	Chapter       ChapterList
	ParentChapter *Chapter
}

func (c *Chapter) Lecture() *Lecture {
	return c.lecture
}

func (c *Chapter) Num() ChapterNum {
	return c.num
}

func (c *Chapter) Tasks() int {
	if c.HasSubChapter() {
		n := 0
		for _, ch := range c.Chapter {
			n += ch.Tasks()
		}
		return n
	}
	return len(c.Task)
}

func (c *Chapter) GetTask(tid TaskNum) (*Task, error) {
	if tid < 0 || int(tid) >= len(c.Task) {
		return nil, fmt.Errorf("task %d not found", tid)
	}
	return c.Task[tid], nil
}

func (c *Chapter) IsEmpty() bool {
	return len(c.Task) == 0 && c.Description == "" && c.Title == ""
}

func (c *Chapter) HasSubChapter() bool {
	return len(c.Chapter) > 0
}

func (c *Chapter) FullTitle() string {
	if c.ParentChapter != nil {
		return c.ParentChapter.FullTitle() + " - " + c.Title
	}
	return c.Title
}

func (c *Chapter) init(cnum ChapterNum, l *Lecture) error {
	if c.Title == "" {
		return fmt.Errorf("no title in chapter %d", cnum)
	}
	c.lecture = l
	c.num = cnum
	c.Description = cleanUpMarkdown(c.Description)

	if len(c.Task) > 0 && c.HasSubChapter() {
		return fmt.Errorf("chapter '%s' contains both tasks and subchapters", c.Title)
	}

	if c.HasSubChapter() {
		for cnu, ch := range c.Chapter {
			ch.ParentChapter = c
			err := ch.init(append(cnum[0:len(cnum):len(cnum)], cnu), l)
			if err != nil {
				return err
			}
		}
	} else {
		for tNum, task := range c.Task {
			task.chapter = c
			task.num = TaskNum(tNum)
			task.Question = cleanUpMarkdown(task.Question)

			if task.Name == "" {
				task.Name = fmt.Sprintf("Frage %d", tNum+1)
			} else {
				task.Name = fmt.Sprintf("Frage %d: %s", tNum+1, task.Name)
			}

			if len(task.Input) == 0 {
				return fmt.Errorf("no input in chapter '%s' task '%s'", c.Title, task.Name)
			}

			vars := make(map[InputId]InputType)
			for _, i := range task.Input {
				i.Label = cleanUpMarkdown(i.Label)

				if i.Id == "" {
					return fmt.Errorf("no id at input in chapter '%s' task '%s'", c.Title, task.Name)
				}

				if err := checkIdent(string(i.Id)); err != nil {
					return fmt.Errorf("invalid id '%s' at input in chapter '%s' task '%s': %w", i.Id, c.Title, task.Name, err)
				}

				if _, ok := vars[i.Id]; ok {
					return fmt.Errorf("duplicate input id '%s' in chapter '%s' task '%s'", i.Id, c.Title, task.Name)
				}
				vars[i.Id] = i.Type

				if i.Label == "" {
					return fmt.Errorf("no label at input id '%s' in chapter '%s' task '%s'", i.Id, c.Title, task.Name)
				}
			}

			hasValidator := make(map[InputId]bool)
			var needsToBeUsedInTaskValidator []InputId
			for _, i := range task.Input {
				if i.Validator != nil {
					err := i.Validator.init(vars, []InputId{i.Id})
					if err != nil {
						return fmt.Errorf("invalid expression in input id '%s' in chapter '%s' task '%s': %w", i.Id, c.Title, task.Name, err)
					}
					hasValidator[i.Id] = true
				} else {
					needsToBeUsedInTaskValidator = append(needsToBeUsedInTaskValidator, i.Id)
				}

				if task.Validator == nil && i.Validator == nil {
					return fmt.Errorf("validator is missing in input id '%s' in chapter '%s' task '%s'", i.Id, c.Title, task.Name)
				}
			}
			task.inputHasValidator = hasValidator

			if task.Validator != nil {
				err := task.Validator.init(vars, needsToBeUsedInTaskValidator)
				if err != nil {
					return fmt.Errorf("invalid expression in chapter '%s' task '%s': %w", c.Title, task.Name, err)
				}
			} else {
				if len(needsToBeUsedInTaskValidator) > 0 {
					return fmt.Errorf("validator is missing in chapter '%s' task '%s'", c.Title, task.Name)
				}
			}
			task.tid = task.createId()
		}
	}
	return nil
}

func (c *Chapter) Iter(yield func(task *Task) bool) {
	c.iter(yield)
}

func (c *Chapter) iter(yield func(task *Task) bool) bool {
	if c.HasSubChapter() {
		for _, ch := range c.Chapter {
			if !ch.iter(yield) {
				return false
			}
		}
		return true
	} else {
		for _, t := range c.Task {
			if !yield(t) {
				return false
			}
		}
		return true
	}
}

type ChapterList []*Chapter

func (c ChapterList) _get(num ChapterNum) *Chapter {
	if len(num) == 0 {
		return nil
	}

	cn := num[0]
	if cn < 0 || cn >= len(c) {
		return nil
	}

	if len(num) == 1 {
		return c[cn]
	}

	return c[cn].Chapter._get(num[1:])
}

type Lecture struct {
	Id          LectureId `xml:"id,attr"`
	Title       string
	Author      string
	AuthorEMail string
	Description string
	Chapter     ChapterList
	folder      string
	files       map[string][]byte
}

func (l *Lecture) TaskCount() int {
	n := 0
	for _, c := range l.Chapter {
		n += c.Tasks()
	}
	return n
}

func (l *Lecture) LID() LectureId {
	return l.Id
}

func (l *Lecture) Iter(yield func(task *Task) bool) {
	for _, c := range l.Chapter {
		if !c.iter(yield) {
			return
		}
	}
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
	if err := checkIdent(string(l.Id)); err != nil {
		return fmt.Errorf("invalid lecture id '%s' in lecturer '%s': %w", l.Id, l.Title, err)
	}

	if l.Title == "" {
		return errors.New("lecture has no title")
	}
	if l.Author == "" {
		return fmt.Errorf("author is missing in lecture %s", l.Title)
	}
	if l.AuthorEMail == "" {
		return fmt.Errorf("author email is missing in lecture %s", l.Title)
	}

	l.Description = cleanUpMarkdown(l.Description)

	err := l.resolveIncludes(l.Chapter)
	if err != nil {
		return err
	}

	for cnum, chapter := range l.Chapter {
		err = chapter.init(ChapterNum{cnum}, l)
		if err != nil {
			return fmt.Errorf("error in lecture '%s', chapter %d: %w", l.Title, cnum+1, err)
		}
	}

	log.Printf("lecture '%s' (id=%s) initialized with %d tasks and %d images", l.Title, l.Id, l.TaskCount(), len(l.files))
	return nil
}

func (l *Lecture) resolveIncludes(chapters []*Chapter) error {
	for i, c := range chapters {
		if c.Include != "" {
			if !c.IsEmpty() {
				return fmt.Errorf("chapter referencing %s contains also other data which is ignored", c.Include)
			}

			fi, ok := l.files[c.Include]
			if !ok {
				return fmt.Errorf("chapter reference %s not found", c.Include)
			}

			var ch Chapter
			err := xml.Unmarshal(fi, &ch)
			if err != nil {
				return err
			}

			if ch.Include != "" {
				return fmt.Errorf("chapter %s contains reference to chapter %s", c.Include, ch.Include)
			}

			chapters[i] = &ch
			delete(l.files, c.Include)
		}

		if chapters[i].HasSubChapter() {
			err := l.resolveIncludes(chapters[i].Chapter)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

// cleanUpMarkdown removes leading spaces from lines.
// Avoids markdown rendering of code.
func cleanUpMarkdown(md string) string {
	if md == "" {
		return ""
	}

	md = strings.TrimLeft(md, "\n")
	md = strings.ReplaceAll(md, "\t", "    ") // tab means 4 spaces

	lines := strings.Split(md, "\n")
	if len(lines) <= 1 {
		return strings.TrimSpace(md)
	}

	// remove trailing spaces
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}

	spaces := math.MaxInt32
	for _, line := range lines {
		if len(line) > 0 {
			s := strings.IndexFunc(line, func(r rune) bool {
				return r != ' '
			})
			if s < spaces {
				spaces = s
			}
		}
	}
	sb := strings.Builder{}
	for i, line := range lines {
		if len(line) > 0 {
			sb.WriteString(line[spaces:])
		}
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	r := sb.String()
	return r
}

func checkIdent(id string) error {
	for i, c := range id {
		if i == 0 {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				continue
			}
			return fmt.Errorf("invalid character '%c' at position %d", c, i+1)
		} else {
			if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
				continue
			}
			return fmt.Errorf("invalid character '%c' at position %d", c, i+1)
		}
	}
	return nil
}

func (l *Lecture) GetChapter(num ChapterNum) (*Chapter, error) {
	c := l.Chapter._get(num)
	if c == nil {
		return nil, fmt.Errorf("chapter %v not found in '%s'", num, l.Title)
	}
	return c, nil
}

func (l *Lecture) GetTask(ch ChapterNum, tn TaskNum) (*Task, error) {
	chapter, err := l.GetChapter(ch)
	if err != nil {
		return nil, err
	}
	return chapter.GetTask(tn)
}

func (l *Lecture) TasksInChapter(cnum int) int {
	if cnum < 0 || cnum >= len(l.Chapter) {
		return 0
	}
	return len(l.Chapter[cnum].Task)
}

func (l *Lecture) CanReload() bool {
	return l.folder != ""
}

func (l *Lecture) HasTask(tid TaskId) bool {
	for task := range l.Iter {
		if task.tid == tid {
			return true
		}
	}
	return false
}

type Lectures struct {
	rwMutex  sync.RWMutex
	lectures map[LectureId]*Lecture
	list     []*Lecture
	folder   string
}

func (l *Lectures) Insert(lecture *Lecture) {
	l.rwMutex.Lock()
	defer l.rwMutex.Unlock()

	if l.lectures == nil {
		l.lectures = make(map[LectureId]*Lecture)
	}

	l.lectures[lecture.Id] = lecture
	l.init()
}

func (l *Lectures) init() {
	lectureList := make([]*Lecture, 0, len(l.lectures))
	for _, lecture := range l.lectures {
		lectureList = append(lectureList, lecture)
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

func (l *Lectures) GetLecture(id LectureId) (*Lecture, error) {
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
		l.lectures = make(map[LectureId]*Lecture)
	}
	l.lectures[lecture.Id] = lecture
}

func (l *Lectures) Uploaded(file []byte) error {
	lecture, err := ReadZip(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(l.folder, string(lecture.Id)+".zip"))
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(file)

	l.Insert(lecture)

	return nil
}

func (l *Lectures) Reload(id LectureId) (*Lecture, error) {
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

type DataMap map[InputId]interface{}

func (d DataMap) Get(key string) (value.Value, bool) {
	v, ok := d[InputId(key)]
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
			if !yield(string(k), v) {
				return false
			}
		}
	}
	return true
}

func (d DataMap) Size() int {
	return len(d)
}

func (t *Task) Validate(input DataMap, showResult bool) map[InputId]string {
	m := value.NewMap(input)
	result := make(map[InputId]string)
	t.Validator.ToResultMap(m, "_task_", result, showResult)
	for _, i := range t.Input {
		i.Validator.ToResultMap(m, i.Id, result, showResult)
	}

	return result
}

func (t *Task) createId() TaskId {
	h := sha1.New()
	h.Write([]byte(t.Name))
	h.Write([]byte(t.Question))
	for _, i := range t.Input {
		h.Write([]byte(i.Id))
		h.Write([]byte(i.Label))
	}
	return TaskId(fmt.Sprintf("%x", h.Sum(nil)))
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

type complexityVisitor struct {
	n int
}

func (c *complexityVisitor) Visit(ast parser2.AST) bool {
	switch a := ast.(type) {
	case *parser2.FunctionCall:
		c.n++
		for _, arg := range a.Args {
			arg.Traverse(c)
		}
		return false
	case *parser2.Const[float64]:
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

func createExpressionMethods(parser *parser2.Parser[float64]) value.MethodMap {
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
		"complexity": value.MethodAtType(0, func(e Expression, stack funcGen.Stack[value.Value]) (value.Value, error) {
			ast, err := parser.Parse(e.expression)
			if err != nil {
				return nil, GuiError{message: "Fehler im Ausdruck '" + e.expression + "'", cause: err}
			}
			v := complexityVisitor{}
			ast.Traverse(&v)
			return value.Int(v.n), nil
		}),
		"mathMl": value.MethodAtType(0, func(e Expression, stack funcGen.Stack[value.Value]) (value.Value, error) {
			ast, err := parser.Parse(e.expression)
			if err != nil {
				return nil, GuiError{message: "Fehler im Ausdruck '" + e.expression + "'", cause: err}
			}
			a, err := MathMlFromAST(ast)
			if err != nil {
				return nil, err
			}
			sb := strings.Builder{}
			sb.WriteString("<math xmlns='http://www.w3.org/1998/Math/MathML'>")
			a.ToMathMl(&sb, nil)
			sb.WriteString("</math>")
			return value.String(sb.String()), nil
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
		f.AddStaticFunction("funcCplx", funcGen.Function[value.Value]{
			Func:   value.Must(f.GenerateFromString(`parseFunc(f,vars).complexity()`, "f", "vars")),
			Args:   2,
			IsPure: true,
		}.SetDescription("func", "argList",
			"returns the complexity of a given function."))
		f.AddStaticFunction("cmpFuncCplx", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`if cmpFunc(exp,is,vars,values)
                                                        then
                                                          if funcCplx(exp,vars)>=funcCplx(is,vars) 
                                                          then true
                                                          else "Der Ausdruck ist zwar korrekt, aber nicht vollständig vereinfacht!"
                                                        else "Der Ausdruck ist nicht korrekt!"`, "exp", "is", "vars", "values")),
			Args:   4,
			IsPure: true,
		}.SetDescription("expected func", "actual func", "argList", "values",
			"compares two functions by evaluating them for a list of arguments.\n"+
				"It returns true if the difference between the two functions is less than 0.0001 for all arguments and "+
				"the complexity of the actual function is equal or less compared to the expected function."))
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
		f.AddStaticFunction("cmpValuesAbs", funcGen.Function[value.Value]{
			Func: value.Must(f.GenerateFromString(`let isExp=parseFunc(isStr,[]);
                                                    let is=abs(isExp.eval([]));
													if expected=0 
                                                    then abs(is)<percent/100
                                                    else
                                                      let dif=abs((is-expected)/expected*100);
                                                      dif<percent`, "expected", "isStr", "percent")),
			Args:   3,
			IsPure: true,
		}.SetDescription("expected", "is", "percent",
			"compares two values and returns true if the difference is less than the given percent of the expected value"))

		f.RegisterMethods(ExpressionTypeId, createExpressionMethods(floatParser.GetParser()))

		p := f.GetParser()
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
