package session

import (
	"bytes"
	"context"
	"github.com/hneemann/objectDB/serialize"
	"github.com/hneemann/quiz/data"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

type Session struct {
	mutex        sync.Mutex
	time         time.Time
	completed    map[string]map[data.InnerId]bool
	persistToken string
	dataModified bool
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
		s.completed = make(map[string]map[data.InnerId]bool)
	}

	lmap, ok := s.completed[id.LHash]
	if !ok {
		lmap = make(map[data.InnerId]bool)
		s.completed[id.LHash] = lmap
	}

	s.dataModified = true
	lmap[id.InnerId] = true
}

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

func (s *Session) ChapterCompleted(hash string, cid int) int {
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
	log.Println("persisted session data", s.persistToken)
}

func (s *Session) restore(path string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	fileData, err := os.ReadFile(path)
	if err != nil {
		log.Println("error reading session data", err)
		return
	}
	s.completed = make(map[string]map[data.InnerId]bool)
	err = serialize.New().Read(bytes.NewReader(fileData), &s.completed)
	if err != nil {
		log.Println("error unmarshal session data", err)
	}
}

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
			log.Println("cleaning sessions")
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

func (s *Sessions) create(persistToken string, w http.ResponseWriter) *Session {
	token := createRandomString()
	session := &Session{persistToken: persistToken}
	session.restore(path.Join(s.dataFolder, session.persistToken))
	session.cleanup(s.lectures)
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

func (s *Sessions) PersistAll() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	log.Println("persisting all sessions")
	for _, session := range s.sessions {
		session.persist(path.Join(s.dataFolder, session.persistToken))
	}
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
			ses = s.create("helmut", w)
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
