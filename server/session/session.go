package session

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/hneemann/objectDB/serialize"
	"github.com/hneemann/quiz/data"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	mutex        sync.Mutex
	time         time.Time
	admin        bool
	completed    map[data.LectureId]map[data.TaskId]bool
	persistToken string
	dataModified bool
}

func (s *Session) touch() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.time = time.Now()
}

func (s *Session) IsAdmin() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.admin
}

// TaskCompleted marks a task as completed.
func (s *Session) TaskCompleted(task *data.Task) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		s.completed = make(map[data.LectureId]map[data.TaskId]bool)
	}

	lectureId := task.Chapter().Lecture().Id
	lmap, ok := s.completed[lectureId]
	if !ok {
		lmap = make(map[data.TaskId]bool)
		s.completed[lectureId] = lmap
	}

	if !lmap[task.TID()] {
		s.dataModified = true
		lmap[task.TID()] = true
	}
}

// IsTaskCompleted returns true if the task is completed.
func (s *Session) IsTaskCompleted(task *data.Task) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return false
	}
	tmap, ok := s.completed[task.Chapter().Lecture().Id]
	if !ok {
		return false
	}
	_, ok = tmap[task.TID()]
	return ok
}

// TasksCompleted returns the number of tasks completed in a chapter.
func (s *Session) TasksCompleted(chapter *data.Chapter) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return 0
	}
	lmap, ok := s.completed[chapter.Lecture().Id]
	if !ok {
		return 0
	}
	count := 0
	for _, t := range chapter.Task {
		if _, ok := lmap[t.TID()]; ok {
			count++
		}
	}

	return count
}

func (s *Session) persist(path string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return
	}

	if !s.dataModified {
		return
	}

	var b bytes.Buffer
	err := serialize.New().Write(&b, s.completed)
	if err != nil {
		log.Println("error serializing session data", err)
		return
	}

	err = os.WriteFile(path, b.Bytes(), 0644)
	if err != nil {
		log.Println("error writing session data", err)
		return
	}

	s.dataModified = false
	log.Println("persisted session", s.persistToken)
}

func (s *Session) restore(path string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fileData, err := os.ReadFile(path)
	if err != nil {
		log.Println("error reading session data", err)
		return
	}
	s.completed = make(map[data.LectureId]map[data.TaskId]bool)
	err = serialize.New().Read(bytes.NewReader(fileData), &s.completed)
	if err != nil {
		log.Println("error unmarshal session data", err)
	}
}

// cleanup removes all completed tasks that are not in the lecture list.
// This is necessary because the lecture list can change.
// If not cleaned up, the session data would contain tasks that do not exist anymore.
func (s *Session) cleanup(lectures *data.Lectures) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return
	}

	for _, lec := range lectures.List() {
		if lmap, ok := s.completed[lec.LID()]; ok {
			for tid := range lmap {
				if !lec.HasTask(tid) {
					delete(lmap, tid)
					s.dataModified = true
				}
			}
		}
	}

}

type Sessions struct {
	mutex      sync.Mutex
	sessions   map[string]*Session
	dataFolder string
	lectures   *data.Lectures
}

const (
	sessionTimeoutMinutes         = 60
	sessionCleanupIntervalMinutes = 20
	maxSessionAge                 = time.Hour * 24 * 180 // after 180 days the session data is removed
)

// New creates a new session manager.
// The dataFolder is the folder where the session data is stored.
// The lectures are used to cleanup the session data by removing completed
// tasks that are not in the lecture list anymore. This is necessary because
// the lecture hash is used to identify the lecture and the hash can change
// if the lecture is modified.
func New(dataFolder string, lectures *data.Lectures) *Sessions {

	if _, err := os.Stat(dataFolder); err != nil {
		err = os.MkdirAll(dataFolder, 0755)
		if err != nil {
			log.Fatal("error creating data folder", err)
		}
	}

	s := &Sessions{
		dataFolder: dataFolder,
		lectures:   lectures,
		sessions:   map[string]*Session{},
	}

	go func() {
		for {
			time.Sleep(sessionCleanupIntervalMinutes * time.Minute)
			var sl []*Session
			s.mutex.Lock()
			for k, v := range s.sessions {
				if time.Since(v.time) > sessionTimeoutMinutes*time.Minute {
					delete(s.sessions, k)
					sl = append(sl, v)
				}
			}
			s.mutex.Unlock()
			for _, ses := range sl {
				ses.persist(path.Join(s.dataFolder, ses.persistToken))
			}
		}
	}()

	return s
}

// Stats returns the statistics for a lecture.
// All stored session data is scanned for the given lecture hash.
// This function also removes old session data.
// All session files are reloaded from disc to avoid data races with
// active sessions
func (s *Sessions) Stats(lid data.LectureId) ([]map[data.TaskId]bool, error) {
	s.PersistAll()

	list, err := os.ReadDir(s.dataFolder)
	if err != nil {
		return nil, err
	}
	var found []map[data.TaskId]bool
	for _, f := range list {
		if !f.IsDir() {
			filePath := filepath.Join(s.dataFolder, f.Name())
			if fi, err := f.Info(); err == nil {
				if time.Since(fi.ModTime()) > maxSessionAge {
					err = os.Remove(filePath)
					if err != nil {
						log.Println("error removing old session file", err)
					}
					continue
				}
			}

			se := &Session{}
			se.restore(filePath)
			if c, ok := se.completed[lid]; ok {
				found = append(found, c)
			}
		}
	}
	return found, nil
}

const cookieName = "sessionId"

func (s *Sessions) get(r *http.Request) (*Session, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	cookie, err := r.Cookie(cookieName)
	if err == nil {
		session, ok := s.sessions[cookie.Value]
		if ok {
			session.touch()
			return session, true
		}
	}
	return nil, false
}

func (s *Sessions) logout(w http.ResponseWriter, r *http.Request) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	cookie, err := r.Cookie(cookieName)
	if err == nil {
		session, ok := s.sessions[cookie.Value]
		if ok {
			session.persist(path.Join(s.dataFolder, session.persistToken))
			delete(s.sessions, cookie.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
}

// Create creates a new session identified by the persistToken which is used as a file name.
func (s *Sessions) Create(persistToken string, admin bool, w http.ResponseWriter) *Session {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for sessionToken, session := range s.sessions {
		if session.persistToken == persistToken {
			log.Println("restoring session", persistToken)
			session.touch()
			http.SetCookie(w, &http.Cookie{
				Name:  cookieName,
				Value: sessionToken,
				Path:  "/",
			})
			return session
		}
	}

	sessionToken := createRandomString()
	session := &Session{persistToken: persistToken, admin: admin}
	session.restore(path.Join(s.dataFolder, session.persistToken))
	session.cleanup(s.lectures)
	session.touch()
	http.SetCookie(w, &http.Cookie{
		Name:  cookieName,
		Value: sessionToken,
		Path:  "/",
	})

	s.sessions[sessionToken] = session

	return session
}

// PersistAll persists all session data.
func (s *Sessions) PersistAll() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, session := range s.sessions {
		session.persist(path.Join(s.dataFolder, session.persistToken))
	}
	log.Println("persisted all sessions")
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func createRandomString() string {
	b := make([]byte, 30)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

const Key = "session"

// Wrap wraps a http.Handler and adds the session data to the context.
func (s *Sessions) Wrap(parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ses, ok := s.get(r)
		if ok {
			c := context.WithValue(r.Context(), Key, ses)
			parent.ServeHTTP(w, r.WithContext(c))
		} else {
			log.Println("redirect to login:", r.URL.Path)
			url := base64.URLEncoding.EncodeToString([]byte(r.URL.Path))
			http.Redirect(w, r, "/login?t="+url, http.StatusFound)
		}
	})
}

// WrapAdmin wraps a http.Handler and adds the session data to the context.
// If the session is not an admin session, the user is redirected to the login page.
func (s *Sessions) WrapAdmin(parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ses, ok := s.get(r)
		if ok && ses.IsAdmin() {
			c := context.WithValue(r.Context(), Key, ses)
			parent.ServeHTTP(w, r.WithContext(c))
		} else {
			log.Println("redirect to login:", r.URL.Path)
			url := base64.URLEncoding.EncodeToString([]byte(r.URL.Path))
			http.Redirect(w, r, "/login?t="+url, http.StatusFound)
		}
	})
}

// LoginData is used to render the login page.
type LoginData struct {
	Target string
	Error  error
}

// Authenticator is used to authenticate a user.
// The Authenticate method returns the user id and a flag if the user is an admin.
type Authenticator interface {
	Authenticate(user, pass string) (string, bool, error)
}

// AuthFunc is a function that implements the Authenticator interface.
type AuthFunc func(user, pass string) (string, bool, error)

func (a AuthFunc) Authenticate(user, pass string) (string, bool, error) {
	return a(user, pass)
}

// LoginHandler returns a http.HandlerFunc that handles the login.
// The loginTemp is the template used to render the login page.
// The login page must contain a form with the fields username, password and target.
// The auth is the authenticator used to authenticate the user.
func LoginHandler(sessions *Sessions, loginTemp *template.Template, auth Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var err error
		encodedTarget := r.URL.Query().Get("t")
		if r.Method == http.MethodPost {
			user := r.FormValue("username")
			pass := r.FormValue("password")
			encodedTarget = r.FormValue("target")

			var id string
			var admin bool
			if id, admin, err = auth.Authenticate(user, pass); err == nil {
				sessions.Create(id, admin, w)

				url := "/"
				t, err := base64.URLEncoding.DecodeString(encodedTarget)
				if err == nil {
					url = string(t)
				}

				log.Println("redirect to", url)
				http.Redirect(w, r, url, http.StatusFound)
				return
			}
		}

		err = loginTemp.Execute(w, LoginData{
			Target: encodedTarget,
			Error:  err,
		})
		if err != nil {
			log.Println(err)
		}
	}
}

// LogoutHandler returns a http.Handler that logs out the user.
func LogoutHandler(sessions *Sessions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions.logout(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	})
}
