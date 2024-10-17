package server

import (
	"embed"
	"fmt"
	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	"github.com/hneemann/quiz/data"
	"github.com/hneemann/quiz/mathml"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

//go:embed assets/*
var Assets embed.FS

//go:embed templates/*
var templateFS embed.FS

var Templates = template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

var funcMap = template.FuncMap{
	"formatTime": func(t time.Time) string {
		timeStr := t.Format("15:04")
		if timeStr == "00:00" {
			return ""
		}
		return timeStr
	},
	"escape": func(s string) string {
		return url.QueryEscape(s)
	},
	"sub": func(a, b int) int {
		return a - b
	},
	"inc": func(i int) int {
		return i + 1
	},
	"dec": func(i int) int {
		return i - 1
	},
	"markdown":     func(raw string) template.HTML { return fromMarkdown(raw, false) },
	"markdownLine": func(raw string) template.HTML { return fromMarkdown(raw, true) },
}

func fromMarkdown(raw string, skipParagraph bool) template.HTML {
	// create Markdown parser with extensions
	extensions := parser.CommonExtensions |
		parser.AutoHeadingIDs |
		parser.NoEmptyLineBeforeBlock |
		parser.SuperSubscript
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(raw))

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags, RenderNodeHook: createRenderHook(skipParagraph)}
	renderer := html.NewRenderer(opts)

	return template.HTML(markdown.Render(doc, renderer))
}

func createRenderHook(skipParagraph bool) html.RenderNodeFunc {
	return func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
		if _, ok := node.(*ast.Paragraph); ok && skipParagraph {
			return ast.GoToNext, true
		} else if m, ok := node.(*ast.Math); ok {
			doMath(w, m.Literal, false)
			return ast.GoToNext, true
		} else if m, ok := node.(*ast.MathBlock); ok {
			if entering {
				doMath(w, m.Literal, true)
			}
			return ast.GoToNext, true
		}
		return ast.GoToNext, false
	}
}

func doMath(w io.Writer, latex []byte, block bool) {
	a, err := mathml.ParseLaTeX(string(latex))
	if err != nil {
		w.Write([]byte("<b>Error: "))
		html.EscapeHTML(w, []byte(err.Error()))
		w.Write([]byte(" in: "))
		html.EscapeHTML(w, latex)
		w.Write([]byte("</b>"))
	} else {
		if block {
			w.Write([]byte("<math display=\"block\" xmlns=\"&mathml;\">"))
		} else {
			w.Write([]byte("<math xmlns=\"&mathml;\">"))
		}
		a.ToMathMl(w)
		w.Write([]byte("</math>"))
	}
}

var mainTemp = Templates.Lookup("main.html")

func CreateMain(lectures []*data.Lecture) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := mainTemp.Execute(w, lectures)
		if err != nil {
			log.Println(err)
		}
	})
}

var lectureTemp = Templates.Lookup("lecture.html")

func CreateLecture(lectures []*data.Lecture) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lStr := r.URL.Query().Get("l")
		l, err := strconv.Atoi(lStr)
		if err != nil || l < 0 || l >= len(lectures) {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		err = lectureTemp.Execute(w, lectures[l])
		if err != nil {
			log.Println(err)
		}
	})
}

var chapterTemp = Templates.Lookup("chapter.html")

func CreateChapter(lectures []*data.Lecture) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l, err := getNumber(r, "l", len(lectures)-1)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		lecture := lectures[l]
		c, err := getNumber(r, "c", len(lecture.Chapter)-1)
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}

		err = chapterTemp.Execute(w, lecture.Chapter[c])
		if err != nil {
			log.Println(err)
		}
	})
}

func getNumber(r *http.Request, id string, max int) (int, error) {
	lStr := r.URL.Query().Get(id)

	if lStr == "" {
		lStr = r.Form.Get(id)
	}

	l, err := strconv.Atoi(lStr)
	if err != nil {
		return 0, err
	}
	if l < 0 || l > max {
		return 0, fmt.Errorf("invalid number")
	}
	return l, nil
}

var taskTemp = Templates.Lookup("task.html")

type taskData struct {
	Task    *data.Task
	Answers data.DataMap
	Result  map[string]string
}

func (td taskData) GetAnswer(id string) string {
	switch a := td.Answers[id].(type) {
	case bool:
		if a {
			return "checked"
		} else {
			return ""
		}
	case string:
		return a
	}
	return ""
}

func (td taskData) GetResult(id string) string {
	return td.Result[id]
}

func CreateTask(lectures []*data.Lecture) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "error parsing form", http.StatusBadRequest)
				return
			}
		}

		l, err := getNumber(r, "l", len(lectures)-1)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}
		lecture := lectures[l]
		c, err := getNumber(r, "c", len(lecture.Chapter)-1)
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}
		chapter := lecture.Chapter[c]
		t, err := getNumber(r, "t", len(chapter.Task)-1)
		if err != nil {
			http.Error(w, "invalid task number", http.StatusBadRequest)
			return
		}
		task := chapter.Task[t]

		td := taskData{Task: task, Answers: data.DataMap{}}

		if r.Method == http.MethodPost {
			for _, i := range task.Input {
				a := r.Form.Get("input_" + i.Id)
				switch i.Type {
				case data.Checkbox:
					td.Answers[i.Id] = strings.ToLower(a) == "on"
				default:
					td.Answers[i.Id] = a
				}
			}
			showResult := r.Form.Get("showResult") != ""
			td.Result = task.Validate(td.Answers, showResult)
		}

		err = taskTemp.Execute(w, td)
		if err != nil {
			log.Println(err)
		}
	})
}
