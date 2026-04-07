package tool

import (
	"io/fs"
	"os"
	"path/filepath"
)

type FileOperations interface {
	Stat(name string) (os.FileInfo, error)
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	ReadDir(name string) ([]os.DirEntry, error)
	WalkDir(root string, fn fs.WalkDirFunc) error
	CreateTemp(dir string, pattern string) (*os.File, error)
}

type localFileOperations struct{}

func CreateLocalFileOperations() FileOperations {
	return localFileOperations{}
}

func (localFileOperations) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (localFileOperations) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (localFileOperations) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

func (localFileOperations) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (localFileOperations) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

func (localFileOperations) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepathWalkDir(root, fn)
}

func (localFileOperations) CreateTemp(dir string, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

var filepathWalkDir = filepath.WalkDir
