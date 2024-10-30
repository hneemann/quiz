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
	"markdown": func(raw string, LId data.LectureId) template.HTML { return fromMarkdown(raw, LId) },
}

func fromMarkdown(raw string, LId data.LectureId) template.HTML {
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

func createRenderHook(LId data.LectureId) html.RenderNodeFunc {
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
				url := "/image/" + string(LId) + "/" + name
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

		isAdmin := false
		if ses, ok := r.Context().Value(session.Key).(*session.Session); ok {
			isAdmin = ses.IsAdmin()
		}

		data := struct {
			Lectures *data.Lectures
			Admin    bool
		}{
			Lectures: lectures,
			Admin:    isAdmin,
		}

		err := mainTemp.Execute(w, data)
		if err != nil {
			log.Println(err)
		}
	})
}

var lectureTemp = Templates.Lookup("lecture.html")

type lectureData struct {
	Lecture *data.Lecture
	session *session.Session
}

// Completed returns the number of completed tasks in the given chapter
func (cd lectureData) Completed(cid data.ChapterId) int {
	if cd.session == nil {
		return 0
	}

	ch, err := cd.Lecture.GetChapter(cid)
	if err != nil {
		return 0
	}

	return cd.session.TasksCompleted(ch)
}

func CreateLecture(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lectureId, _ := getLectureFromPath(r.URL.Path)
		lecture, err := lectures.GetLecture(lectureId)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		ses, _ := r.Context().Value(session.Key).(*session.Session)

		err = lectureTemp.Execute(w, lectureData{Lecture: lecture, session: ses})
		if err != nil {
			log.Println(err)
		}
	})
}

func getStrFromPath(p string) (string, string) {
	if p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	return path.Base(p), path.Dir(p)
}

func getLectureFromPath(p string) (data.LectureId, string) {
	str, n := getStrFromPath(p)
	return data.LectureId(str), n
}

func getTaskFromPath(p string) (data.TaskId, string) {
	str, n := getStrFromPath(p)
	return data.TaskId(str), n
}

func getIntFromPath(p string) (int, string) {
	str, n := getStrFromPath(p)
	i, err := strconv.Atoi(str)
	if err != nil {
		return -1, n
	}
	return i, n
}

var chapterTemp = Templates.Lookup("chapter.html")

type chapterData struct {
	Chapter *data.Chapter
	session *session.Session
	state   data.LectureState
}

func (cd chapterData) Completed(id data.AbsTaskId) bool {
	if cd.session == nil {
		return false
	}
	return cd.session.IsTaskCompleted(id)
}

func (cd chapterData) IsAvail(id data.AbsTaskId) bool {
	return IsTaskAvail(id, cd.Chapter, &cd.state, cd.session)
}

func IsTaskAvail(id data.AbsTaskId, c *data.Chapter, state *data.LectureState, session *session.Session) bool {
	if state.ShowAllTasks || c.IsFirstTask(id.TaskId) || !c.StepByStep {
		return true
	}

	if session == nil {
		return false
	}

	if session.IsAdmin() {
		return true
	}

	return session.IsTaskCompleted(data.AbsTaskId{
		LectureId: id.LectureId,
		TaskId:    c.GetTaskBefore(id.TaskId),
	})
}

func CreateChapter(lectures *data.Lectures, states *data.LectureStates) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, next := getIntFromPath(r.URL.Path)
		l, _ := getLectureFromPath(next)
		lecture, err := lectures.GetLecture(l)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		chapter, err := lecture.GetChapter(data.ChapterId(c))
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}

		ses, _ := r.Context().Value(session.Key).(*session.Session)

		err = chapterTemp.Execute(w, chapterData{Chapter: chapter, session: ses, state: states.Get(lecture.Id)})
		if err != nil {
			log.Println(err)
		}
	})
}

var taskTemp = Templates.Lookup("task.html")

type taskData struct {
	HasResult           bool
	ShowSolutionsButton bool
	Task                *data.Task
	Answers             data.DataMap
	Result              map[data.InputId]string
	Next                string
	Ok                  bool
	ShowReload          bool
	ReloadError         error
}

func (td *taskData) GetAnswer(id data.InputId) string {
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

func (td *taskData) GetResult(id data.InputId) string {
	return td.Result[id]
}

func (td *taskData) HasHook(id data.InputId) bool {
	if !td.HasResult {
		return false
	}

	if !td.Task.InputHasValidator(id) {
		return false
	}

	_, isMessage := td.Result[id]
	return !isMessage
}

func CreateTask(lectures *data.Lectures, states *data.LectureStates) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, next := getTaskFromPath(r.URL.Path)
		c, next := getIntFromPath(next)
		l, _ := getLectureFromPath(next)
		lecture, err := lectures.GetLecture(l)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		var reloadError error
		reload := r.URL.Query().Get("rl") == "true"
		if reload {
			var nl *data.Lecture
			nl, reloadError = lectures.Reload(lecture.Id)
			if nl != nil {
				lecture = nl
			}
		}

		chapter, err := lecture.GetChapter(data.ChapterId(c))
		if err != nil {
			http.Error(w, "invalid chapter number", http.StatusBadRequest)
			return
		}

		task, err := chapter.GetTask(t)
		if err != nil {
			http.Error(w, "invalid task number", http.StatusBadRequest)
			return
		}

		state := states.Get(lecture.Id)
		showSolutions := state.ShowSolutions
		showReload := false
		ses, _ := r.Context().Value(session.Key).(*session.Session)
		if ses != nil {
			showSolutions = showSolutions || ses.IsAdmin()
			showReload = ses.IsAdmin() && lecture.CanReload()

			if !IsTaskAvail(task.GetId(), chapter, &state, ses) {
				http.Error(w, "task not available", http.StatusForbidden)
				return
			}
		}

		td := taskData{Task: task, Answers: data.DataMap{}, ShowSolutionsButton: showSolutions, ShowReload: showReload, ReloadError: reloadError}

		if r.Method == http.MethodPost {
			err = r.ParseForm()
			if err != nil {
				http.Error(w, "error parsing form", http.StatusBadRequest)
				return
			}
			for _, i := range task.Input {
				a := r.Form.Get("input_" + string(i.Id))
				switch i.Type {
				case data.Checkbox:
					td.Answers[i.Id] = strings.ToLower(a) == "on"
				default:
					td.Answers[i.Id] = a
				}
			}
			showResult := showSolutions && r.Form.Get("showResult") != ""
			td.Result = task.Validate(td.Answers, showResult)
			if len(td.Result) == 0 {

				if ses != nil {
					ses.TaskCompleted(task.GetId())
				}

				td.Ok = true
			}
			td.HasResult = true
		}

		if ses != nil && ses.IsTaskCompleted(task.GetId()) {
			if ntid, ok := chapter.IsTaskBehind(task.TID()); ok {
				td.Next = fmt.Sprintf("/task/%s/%d/%s/", task.LID(), chapter.CID(), ntid)
			}
		}

		err = taskTemp.Execute(w, &td)
		if err != nil {
			log.Println(err)
		}
	})
}

func CreateImages(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, next := getStrFromPath(r.URL.Path)
		l, _ := getLectureFromPath(next)

		lecture, err := lectures.GetLecture(l)
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

var adminTemp = Templates.Lookup("admin.html")

func CreateAdmin(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseMultipartForm(32 << 20)
			if err != nil {
				respondWithError(w, err)
				return
			}

			file, _, err := r.FormFile("file")
			if err != nil {
				respondWithError(w, err)
				return
			}
			zip, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				respondWithError(w, err)
				return
			}

			err = lectures.Uploaded(zip)
			if err != nil {
				respondWithError(w, err)
				return
			}
		}
		err := adminTemp.Execute(w, lectures)
		if err != nil {
			log.Println(err)
		}
	})
}

var errorViewTemp = Templates.Lookup("error.html")

func respondWithError(writer http.ResponseWriter, e error) {
	log.Println(e)
	err := errorViewTemp.Execute(writer, e)
	if err != nil {
		log.Println(err)
	}
}

var statsViewTemp = Templates.Lookup("statistics.html")

type StatsData struct {
	Title   string
	Chapter []StatsChapter
}
type StatsChapter struct {
	Title string
	Task  []StatsTask
}
type StatsTask struct {
	Task  string
	Count int
}

func CreateStatistics(lectures *data.Lectures, sessions *session.Sessions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := data.LectureId(r.URL.Query().Get("id"))
		lecture, err := lectures.GetLecture(id)
		if err != nil {
			http.Redirect(w, r, "/admin", http.StatusFound)
			return
		}

		statsMap, err := sessions.Stats(lecture.LID())
		if err != nil {
			http.Error(w, "error collecting data", http.StatusInternalServerError)
			return
		}

		stats := StatsData{Title: lecture.Title}
		for _, c := range lecture.Chapter {
			chapter := StatsChapter{Title: c.Title}
			for _, t := range c.Task {
				counter := 0
				for _, s := range statsMap {
					if comp, ok := s[t.TID()]; ok {
						if comp {
							counter++
						}
					}
				}
				chapter.Task = append(chapter.Task, StatsTask{Task: t.Name, Count: counter})
			}
			stats.Chapter = append(stats.Chapter, chapter)
		}

		err = statsViewTemp.Execute(w, stats)
		if err != nil {
			log.Println(err)
		}
	})
}

var settingsTemp = Templates.Lookup("settings.html")

type settingsData struct {
	Title    string
	Settings data.LectureState
}

func CreateSettings(lectures *data.Lectures, states *data.LectureStates) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := getLectureFromPath(r.URL.Path)
		lecture, err := lectures.GetLecture(id)
		if err != nil {
			http.Error(w, "invalid lecture number", http.StatusBadRequest)
			return
		}

		settings := states.Get(id)

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				respondWithError(w, err)
				return
			}

			settings.ShowSolutions = r.Form.Get("showSolutions") == "true"
			settings.ShowAllTasks = r.Form.Get("showAllTasks") == "true"

			err = states.SetState(id, settings)
			if err != nil {
				respondWithError(w, err)
				return
			}
		}

		err = settingsTemp.Execute(w, settingsData{Title: lecture.Title, Settings: settings})
		if err != nil {
			log.Println(err)
		}
	})
}
