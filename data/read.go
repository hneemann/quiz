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
	lectures := Lectures{folder: folder}
	for _, f := range l {
		name := f.Name()
		if len(name) > 0 && name[0] == '.' {
			continue
		}
		var lecture *Lecture
		if f.IsDir() {
			lecture, err = readFolder(filepath.Join(folder, f.Name()))
			if err != nil {
				return nil, err
			}
		} else if filepath.Ext(name) == ".zip" {
			lecture, err = readZipFile(filepath.Join(folder, f.Name()))
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
	lecture.folder = folder
	return lecture, nil
}
func readZipFile(zipFile string) (*Lecture, error) {
	r, err := os.Open(zipFile)
	if err != nil {
		return nil, fmt.Errorf("error reading zip file %s: %w", zipFile, err)
	}

	fi, err := r.Stat()
	if err != nil {
		return nil, fmt.Errorf("error reading zip file %s: %w", zipFile, err)
	}

	defer r.Close()
	return ReadZip(r, fi.Size())
}

func ReadZip(r io.ReaderAt, size int64) (*Lecture, error) {
	z, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("error reading zip file: %w", err)
	}

	fileData := map[string][]byte{}
	var lecture *Lecture
	for _, f := range z.File {
		if filepath.Ext(f.Name) == ".xml" {
			if lecture != nil {
				return nil, fmt.Errorf("multiple xml files in zip file ")
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
		return nil, fmt.Errorf("no xml file found in zip file")
	}
	lecture.files = fileData
	return lecture, nil
}
