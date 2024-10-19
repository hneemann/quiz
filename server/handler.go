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
	"github.com/hneemann/quiz/server/session"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
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
	"markdown": func(raw string, LId int) template.HTML { return fromMarkdown(raw, LId) },
}

func fromMarkdown(raw string, LId int) template.HTML {
	// create Markdown parser with extensions
	extensions := parser.CommonExtensions |
		parser.AutoHeadingIDs |
		parser.NoEmptyLineBeforeBlock |
		parser.SuperSubscript
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse([]byte(raw))

	if d, ok := doc.(*ast.Document); ok {
		if len(d.Children) == 1 {
			if c, ok := d.Children[0].(*ast.Paragraph); ok {
				d.Children = c.Children
			}
		}
	}

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags, RenderNodeHook: createRenderHook(LId)}
	renderer := html.NewRenderer(opts)

	return template.HTML(markdown.Render(doc, renderer))
}

func createRenderHook(LId int) html.RenderNodeFunc {
	return func(w io.Writer, node ast.Node, entering bool) (ast.WalkStatus, bool) {
		switch n := node.(type) {
		case *ast.Math:
			doMath(w, n.Literal, false)
			return ast.GoToNext, true
		case *ast.MathBlock:
			if entering {
				doMath(w, n.Literal, true)
			}
			return ast.GoToNext, true
		case *ast.Image:
			if entering {
				attr := n.Attribute
				if attr == nil {
					attr = &ast.Attribute{}
				}
				attr.Classes = append(attr.Classes, []byte("task"))
				n.Attribute = attr

				name := string(n.Destination)
				url := "/image/" + strconv.Itoa(LId) + "/" + name
				n.Destination = []byte(url)
			}
			return ast.GoToNext, false
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
		a.ToMathMl(w, nil)
		w.Write([]byte("</math>"))
	}
}

var mainTemp = Templates.Lookup("main.html")

func CreateMain(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := mainTemp.Execute(w, lectures)
		if err != nil {
			log.Println(err)
		}
	})
}

var lectureTemp = Templates.Lookup("lecture.html")

func CreateLecture(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lecture, err := lectures.GetLecture(getNumber(r, "l"))
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}
		err = lectureTemp.Execute(w, lecture)
		if err != nil {
			log.Println(err)
		}
	})
}

var chapterTemp = Templates.Lookup("chapter.html")

type chapterData struct {
	Chapter *data.Chapter
	session *session.Session
}

func (cd chapterData) Completed(id data.TaskId) bool {
	if cd.session == nil {
		return false
	}
	return cd.session.IsTaskCompleted(id)
}

func CreateChapter(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lecture, err := lectures.GetLecture(getNumber(r, "l"))
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		chapter, err := lecture.GetChapter(getNumber(r, "c"))
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}

		ses, _ := r.Context().Value("session").(*session.Session)

		err = chapterTemp.Execute(w, chapterData{Chapter: chapter, session: ses})
		if err != nil {
			log.Println(err)
		}
	})
}

func getNumber(r *http.Request, id string) (int, error) {
	lStr := r.URL.Query().Get(id)

	if lStr == "" {
		lStr = r.Form.Get(id)
	}

	l, err := strconv.Atoi(lStr)
	if err != nil {
		return 0, err
	}
	return l, nil
}

var taskTemp = Templates.Lookup("task.html")

type taskData struct {
	HasResult        bool
	ShowResultButton bool
	Task             *data.Task
	Answers          data.DataMap
	Result           map[string]string
	Next             string
	Ok               bool
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

func CreateTask(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "error parsing form", http.StatusBadRequest)
				return
			}
		}

		lecture, err := lectures.GetLecture(getNumber(r, "l"))
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		chapter, err := lecture.GetChapter(getNumber(r, "c"))
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}

		task, err := chapter.GetTask(getNumber(r, "t"))
		if err != nil {
			http.Error(w, "invalid task number", http.StatusBadRequest)
			return
		}

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
			if len(td.Result) == 0 {

				if ses, ok := r.Context().Value("session").(*session.Session); ok {
					ses.TaskCompleted(task.GetId())
				}

				td.Ok = true
			}
			td.HasResult = true
		}
		if task.TID() < len(chapter.Task)-1 {
			td.Next = fmt.Sprintf("/task?l=%d&c=%d&t=%d", task.LID(), task.CID(), task.TID()+1)
		}

		err = taskTemp.Execute(w, td)
		if err != nil {
			log.Println(err)
		}
	})
}

func CreateImages(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Path
		file := path.Base(url)

		lecture, err := lectures.GetLecture(strconv.Atoi(path.Base(path.Dir(url))))
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}
		data, err := lecture.GetFile(file)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		ctype := mime.TypeByExtension(filepath.Ext(file))
		if ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		w.Write(data)
	})
}
