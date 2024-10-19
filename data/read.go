package data

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func ReadLectures(folder string) (*Lectures, error) {
	l, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("error reading lecture directory: %w", err)
	}
	var lectures Lectures
	for _, f := range l {
		var lecture *Lecture
		if f.IsDir() {
			lecture, err = readFolder(filepath.Join(folder, f.Name()))
			if err != nil {
				return nil, err
			}
		} else if filepath.Ext(f.Name()) == ".zip" {
			lecture, err = ReadZip(filepath.Join(folder, f.Name()))
			if err != nil {
				return nil, err
			}
		}
		if lecture != nil {
			lectures.add(lecture)
		}
	}

	lectures.init()
	return &lectures, nil
}

func readFolder(folder string) (*Lecture, error) {

	files, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("error reading folder %s: %w", folder, err)
	}

	fileData := map[string][]byte{}
	var lecture *Lecture
	for _, f := range files {
		if !f.IsDir() {
			path := filepath.Join(folder, f.Name())
			if filepath.Ext(f.Name()) == ".xml" {
				if lecture != nil {
					return nil, fmt.Errorf("multiple xml files in folder %s", folder)
				}
				file, err := os.Open(path)
				if err != nil {
					return nil, fmt.Errorf("error opening file %s: %w", f.Name(), err)
				}
				defer file.Close()

				lecture, err = New(file)
				if err != nil {
					return nil, fmt.Errorf("error parsing file %s: %w", path, err)
				}
			} else {
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, fmt.Errorf("error reading file %s: %w", path, err)
				}
				fileData[f.Name()] = data
			}
		}
	}
	if lecture == nil {
		return nil, fmt.Errorf("no xml file found in folder %s", folder)
	}
	lecture.files = fileData
	return lecture, nil
}

func ReadZip(zipFile string) (*Lecture, error) {

	z, err := zip.OpenReader(zipFile)
	if err != nil {
		return nil, fmt.Errorf("error reading folder %s: %w", zipFile, err)
	}
	defer z.Close()

	fileData := map[string][]byte{}
	var lecture *Lecture
	for _, f := range z.File {
		if filepath.Ext(f.Name) == ".xml" {
			if lecture != nil {
				return nil, fmt.Errorf("multiple xml files in zip file %s", zipFile)
			}
			file, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening file %s: %w", f.Name, err)
			}
			lecture, err = New(file)
			if err != nil {
				return nil, fmt.Errorf("error parsing file %s: %w", f.Name, err)
			}
		} else {
			zData, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error reading file %s: %w", f.Name, err)
			}
			data, err := io.ReadAll(zData)
			if err != nil {
				return nil, fmt.Errorf("error reading file %s: %w", f.Name, err)
			}
			fileData[f.Name] = data
		}
	}
	if lecture == nil {
		return nil, fmt.Errorf("no xml file found in zip file %s", zipFile)
	}
	lecture.files = fileData
	return lecture, nil
}
