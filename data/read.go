package data

import (
	"fmt"
	"os"
	"path/filepath"
)

func ReadLectures(folder string) (Lectures, error) {
	l, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("error reading lecture directory: %w", err)
	}
	var lectures Lectures
	for _, f := range l {
		if f.IsDir() {
			continue
		}
		if filepath.Ext(f.Name()) == ".xml" {
			lecture, err := readFile(filepath.Join(folder, f.Name()))
			if err != nil {
				return nil, err
			}
			lectures = append(lectures, lecture)
		}
	}
	err = lectures.Init()
	if err != nil {
		return nil, fmt.Errorf("error initializing lectures: %w", err)
	}
	return lectures, nil
}

func readFile(filename string) (*Lecture, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file %s: %w", filename, err)
	}
	defer file.Close()
	lecture, err := New(file)
	if err != nil {
		return nil, fmt.Errorf("error parsing file %s: %w", filename, err)
	}
	lecture.filename = filename
	return lecture, nil
}
