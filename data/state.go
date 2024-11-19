package data

import (
	"bytes"
	"github.com/hneemann/objectDB/serialize"
	"log"
	"os"
	"sync"
)

type LectureState struct {
	ShowSolutions bool
	ShowAllTasks  bool
	Disabled      bool
}

type LectureStates struct {
	mutex  sync.Mutex
	states map[LectureId]LectureState
	path   string
}

func NewLectureStates(path string) *LectureStates {
	ls := LectureStates{path: path, states: make(map[LectureId]LectureState)}
	fileData, err := os.ReadFile(path)
	if err != nil {
		log.Print("could not read state", path)
		return &ls
	}
	err = serialize.New().Read(bytes.NewReader(fileData), &(ls.states))
	if err != nil {
		log.Print("could not deserialize state", path)
	}

	return &ls
}

func (ls *LectureStates) persist() error {
	var b bytes.Buffer
	err := serialize.New().Write(&b, ls.states)
	if err != nil {
		return err
	}

	return os.WriteFile(ls.path, b.Bytes(), 0644)
}

func (ls *LectureStates) Get(id LectureId) LectureState {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	state := ls.states[id]
	return state
}

func (ls *LectureStates) SetState(id LectureId, state LectureState) error {
	ls.mutex.Lock()
	defer ls.mutex.Unlock()

	ls.states[id] = state

	return ls.persist()
}
