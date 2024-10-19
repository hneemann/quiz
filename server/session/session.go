package session

import (
	"context"
	"github.com/hneemann/quiz/data"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	mutex     sync.Mutex
	time      time.Time
	completed map[data.TaskId]struct{}
}

func (s *Session) touch() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.time = time.Now()
}

func (s *Session) TaskCompleted(id data.TaskId) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		s.completed = map[data.TaskId]struct{}{}
	}

	log.Println("completed task", id)

	s.completed[id] = struct{}{}
}

func (s *Session) IsTaskCompleted(id data.TaskId) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.completed == nil {
		return false
	}
	_, ok := s.completed[id]

	log.Println("check task", id, ok)

	return ok
}

type Sessions struct {
	mutex    sync.Mutex
	sessions map[string]*Session
}

func New() *Sessions {
	return &Sessions{
		sessions: map[string]*Session{},
	}
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

func (s *Sessions) create(w http.ResponseWriter) *Session {

	token := createRandomString()
	session := &Session{}
	session.touch()
	http.SetCookie(w, &http.Cookie{
		Name:  cookieName,
		Value: token,
		Path:  "/",
	})

	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sessions[token] = session

	return session
}

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func createRandomString() string {
	b := make([]byte, 30)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func (s *Sessions) Wrap(parent http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ses, ok := s.get(r)
		if !ok {
			ses = s.create(w)
		}
		//if ok {
		c := context.WithValue(r.Context(), "session", ses)
		parent.ServeHTTP(w, r.WithContext(c))
		//} else {
		//	url := base64.URLEncoding.EncodeToString([]byte(r.URL.RawQuery))
		//	log.Println("redirect to login:", url)
		//	http.Redirect(w, r, "/login?url="+url, http.StatusFound)
		//}
	})
}
