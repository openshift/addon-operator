package dev

import (
	"io/fs"
	"path"
)

// Sorts fs.FileInfo objects by basename.
type FileInfosByName []fs.FileInfo

func (x FileInfosByName) Len() int { return len(x) }

func (x FileInfosByName) Less(i, j int) bool {
	iName := path.Base(x[i].Name())
	jName := path.Base(x[j].Name())
	return iName < jName
}
func (x FileInfosByName) Swap(i, j int) { x[i], x[j] = x[j], x[i] }
