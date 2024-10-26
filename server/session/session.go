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
	completed    map[data.LectureHash]map[data.InnerId]bool
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
func (s *Session) TaskCompleted(id data.TaskId) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		s.completed = make(map[data.LectureHash]map[data.InnerId]bool)
	}

	lmap, ok := s.completed[id.LHash]
	if !ok {
		lmap = make(map[data.InnerId]bool)
		s.completed[id.LHash] = lmap
	}

	if !lmap[id.InnerId] {
		s.dataModified = true
		lmap[id.InnerId] = true
	}
}

// IsTaskCompleted returns true if the task is completed.
func (s *Session) IsTaskCompleted(id data.TaskId) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return false
	}
	tmap, ok := s.completed[id.LHash]
	if !ok {
		return false
	}
	_, ok = tmap[id.InnerId]
	return ok
}

// ChapterCompleted returns the number of tasks completed in a chapter.
// The hash is the hash of the lecture and the cid is the chapter id.
func (s *Session) ChapterCompleted(hash data.LectureHash, cid int) int {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return 0
	}
	tmap, ok := s.completed[hash]
	if !ok {
		return 0
	}
	count := 0
	for i := range tmap {
		if i.CId == cid {
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
	s.completed = make(map[data.LectureHash]map[data.InnerId]bool)
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

	for hash := range s.completed {
		hashExists := false
		for _, lecture := range lectures.List() {
			if lecture.Hash() == hash {
				hashExists = true
				break
			}
		}
		if !hashExists {
			s.dataModified = true
			delete(s.completed, hash)
		}
	}
}

type Sessions struct {
	mutex      sync.Mutex
	sessions   map[string]*Session
	dataFolder string
	lectures   *data.Lectures
}

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
			time.Sleep(10 * time.Minute)
			var sl []*Session
			s.mutex.Lock()
			for k, v := range s.sessions {
				if time.Since(v.time) > 30*time.Minute {
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

func (s *Sessions) Stats(hash data.LectureHash) ([]map[data.InnerId]bool, error) {
	s.PersistAll()

	list, err := os.ReadDir(s.dataFolder)
	if err != nil {
		return nil, err
	}
	var found []map[data.InnerId]bool
	for _, f := range list {
		if !f.IsDir() {
			filePath := filepath.Join(s.dataFolder, f.Name())
			if fi, err := f.Info(); err == nil {
				if time.Since(fi.ModTime()) > time.Hour*24*180 {
					err = os.Remove(filePath)
					if err != nil {
						log.Println("error removing old session file", err)
					}
					continue
				}
			}

			se := &Session{}
			se.restore(filePath)
			if c, ok := se.completed[hash]; ok {
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

type LoginData struct {
	Target string
	Error  error
}

type Authenticator interface {
	Authenticate(user, pass string) (string, bool, error)
}

type AuthFunc func(user, pass string) (string, bool, error)

func (a AuthFunc) Authenticate(user, pass string) (string, bool, error) {
	return a(user, pass)
}

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

func LogoutHandler(sessions *Sessions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessions.logout(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	})
}
