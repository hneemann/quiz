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
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed static/*
var Static embed.FS

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

type mainData struct {
	Lectures *data.Lectures
	Logout   bool
	Admin    bool
	states   *data.LectureStates
}

func (md *mainData) Visible(id data.LectureId) bool {
	if md.Admin {
		return true
	}
	state := md.states.Get(id)
	return !state.Disabled
}

func (md *mainData) Hidden(id data.LectureId) bool {
	state := md.states.Get(id)
	return state.Disabled
}

func CreateMain(lectures *data.Lectures, logout bool, states *data.LectureStates) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		isAdmin := false
		if ses, ok := r.Context().Value(session.Key).(*session.Session); ok {
			isAdmin = ses.IsAdmin()
		}

		data := mainData{
			Lectures: lectures,
			Logout:   logout,
			Admin:    isAdmin,
			states:   states,
		}

		err := mainTemp.Execute(w, &data)
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
func (cd lectureData) Completed(cnum data.ChapterNum) int {
	if cd.session == nil {
		return 0
	}

	ch, err := cd.Lecture.GetChapter(cnum)
	if err != nil {
		return 0
	}

	c := 0
	for task := range ch.Iter {
		if cd.session.IsTaskCompleted(task) {
			c++
		}
	}
	return c
}

func CreateLecture(lectures *data.Lectures) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lectureId, _ := getLectureFromPath(r.URL.Path)
		lecture, err := lectures.GetLecture(lectureId)
		if err != nil {
			panic(err)
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

func getIntFromPath(p string) (int, string) {
	str, n := getStrFromPath(p)
	i, err := strconv.Atoi(str)
	if err != nil {
		return -1, n
	}
	return i, n
}

func getTaskNumFromPath(p string) (data.TaskNum, string) {
	i, n := getIntFromPath(p)
	return data.TaskNum(i), n
}

func getChapterNumFromPath(p string) (data.ChapterNum, string) {
	cs, n := getStrFromPath(p)
	cn, err := data.NewChapterNum(cs)
	if err != nil {
		return data.ChapterNum{0}, n
	}
	return cn, n
}

var (
	chapterTemp  = Templates.Lookup("chapter.html")
	mChapterTemp = Templates.Lookup("mChapter.html")
)

type chapterData struct {
	Chapter *data.Chapter
	session *session.Session
	state   data.LectureState
}

func (cd chapterData) Completed(num data.TaskNum) bool {
	if cd.session == nil {
		return false
	}
	task, err := cd.Chapter.GetTask(num)
	if err != nil {
		return false
	}
	return cd.session.IsTaskCompleted(task)
}

func (cd chapterData) CompletedTasks(num int) int {
	if cd.session == nil {
		return 0
	}
	if num < 0 || num >= len(cd.Chapter.Chapter) {
		return 0
	}

	c := 0
	chapter := cd.Chapter.Chapter[num]
	for task := range chapter.Iter {
		if cd.session.IsTaskCompleted(task) {
			c++
		}
	}

	return c
}

func (cd chapterData) IsAvail(num data.TaskNum) bool {
	if cd.session == nil {
		return false
	}
	task, err := cd.Chapter.GetTask(num)
	if err != nil {
		return false
	}
	return IsTaskAvail(task, &cd.state, cd.session)
}

func IsTaskAvail(task *data.Task, state *data.LectureState, session *session.Session) bool {
	if task.Num() == 0 {
		return true
	}

	if state.ShowAllTasks || !task.Chapter().StepByStep {
		return true
	}

	if session == nil {
		return false
	}

	if session.IsAdmin() {
		return true
	}

	beforeTask, _ := task.Chapter().GetTask(task.Num() - 1)
	return session.IsTaskCompleted(beforeTask)
}

func CreateChapter(lectures *data.Lectures, states *data.LectureStates) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cn, next := getChapterNumFromPath(r.URL.Path)
		l, _ := getLectureFromPath(next)
		lecture, err := lectures.GetLecture(l)
		if err != nil {
			panic(err)
		}

		chapter, err := lecture.GetChapter(cn)
		if err != nil {
			panic(err)
		}

		ses, _ := r.Context().Value(session.Key).(*session.Session)

		if chapter.HasSubChapter() {
			err = mChapterTemp.Execute(w, chapterData{Chapter: chapter, session: ses, state: states.Get(lecture.Id)})
		} else {
			err = chapterTemp.Execute(w, chapterData{Chapter: chapter, session: ses, state: states.Get(lecture.Id)})
		}
		if err != nil {
			log.Println(err)
		}
	})
}

var taskTemp = Templates.Lookup("task.html")

type taskData struct {
	Task                *data.Task
	HasResult           bool
	ShowSolutionsButton bool
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
		tn, next := getTaskNumFromPath(r.URL.Path)
		cn, next := getChapterNumFromPath(next)
		l, _ := getLectureFromPath(next)
		lecture, err := lectures.GetLecture(l)
		if err != nil {
			panic(err)
		}

		var reloadError error
		query := r.URL.Query()
		reload := query.Get("rl") == "true"
		if reload {
			var nl *data.Lecture
			nl, reloadError = lectures.Reload(lecture.Id)
			if nl != nil {
				lecture = nl
			}
		}

		task, err := lecture.GetTask(cn, tn)
		if err != nil {
			panic(err)
		}

		state := states.Get(lecture.Id)
		showSolutions := state.ShowSolutions
		showReload := false
		ses, _ := r.Context().Value(session.Key).(*session.Session)
		if ses != nil {
			showSolutions = showSolutions || ses.IsAdmin()
			showReload = ses.IsAdmin() && lecture.CanReload()

			if !IsTaskAvail(task, &state, ses) {
				panic("task not available")
			}
		}

		td := taskData{
			Task:                task,
			Answers:             data.DataMap{},
			ShowSolutionsButton: showSolutions,
			ShowReload:          showReload,
			ReloadError:         reloadError,
		}

		if r.Method == http.MethodPost {
			err = r.ParseForm()
			if err != nil {
				panic(err)
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
					ses.TaskCompleted(task)
				}

				td.Ok = true
			}
			td.HasResult = true
		}

		if ses != nil && ses.IsTaskCompleted(task) {
			if nTask, err := task.Chapter().GetTask(tn + 1); err == nil {
				td.Next = fmt.Sprintf("/task/%s/%v/%d/", lecture.Id, cn, nTask.Num())
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
			panic(err)
		}
		data, err := lecture.GetFile(file)
		if err != nil {
			panic(err)
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
				panic(err)
			}

			file, _, err := r.FormFile("file")
			if err != nil {
				panic(err)
			}
			zip, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				panic(err)
			}

			err = lectures.Uploaded(zip)
			if err != nil {
				panic(err)
			}
		}
		err := adminTemp.Execute(w, lectures)
		if err != nil {
			log.Println(err)
		}
	})
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
			panic(err)
		}

		statsMap, err := sessions.Stats(lecture.LID())
		if err != nil {
			panic(err)
		}

		stats := StatsData{Title: lecture.Title}
		collectChapter(lecture.Chapter, statsMap, &stats, time.Now().AddDate(0, -6, 0).Unix())

		err = statsViewTemp.Execute(w, stats)
		if err != nil {
			log.Println(err)
		}
	})
}

func collectChapter(chap data.ChapterList, statsMap []map[data.TaskId]int64, stats *StatsData, oldest int64) {
	for _, c := range chap {
		if c.HasSubChapter() {
			collectChapter(c.Chapter, statsMap, stats, oldest)
		} else {
			chapter := StatsChapter{Title: c.Title}
			for _, t := range c.Task {
				counter := 0
				for _, s := range statsMap {
					if date, ok := s[t.TID()]; ok {
						if date > oldest {
							counter++
						}
					}
				}
				chapter.Task = append(chapter.Task, StatsTask{Task: t.Name, Count: counter})
			}
			stats.Chapter = append(stats.Chapter, chapter)
		}
	}
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
			panic(err)
		}

		settings := states.Get(id)

		if r.Method == http.MethodPost {
			err := r.ParseForm()
			if err != nil {
				panic(err)
			}

			settings.ShowSolutions = r.Form.Get("showSolutions") == "true"
			settings.ShowAllTasks = r.Form.Get("showAllTasks") == "true"
			settings.Disabled = r.Form.Get("disabled") == "true"

			err = states.SetState(id, settings)
			if err != nil {
				panic(err)
			}
		}

		err = settingsTemp.Execute(w, settingsData{Title: lecture.Title, Settings: settings})
		if err != nil {
			log.Println(err)
		}
	})
}

var logsTemp = Templates.Lookup("logs.html")

func CreateLogs(folder string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logFile := r.URL.Query().Get("l")
		if logFile != "" {
			if strings.Contains(logFile, "..") {
				panic("invalid log file")
			}
			http.ServeFile(w, r, filepath.Join(folder, logFile))
			return
		}

		entries, err := os.ReadDir(folder)
		if err != nil {
			panic(err)
		}

		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)

		err = logsTemp.Execute(w, names)
		if err != nil {
			log.Println(err)
		}
	})
}
