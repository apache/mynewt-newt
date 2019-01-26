package util

import (
	"fmt"
)

// FileInfo represents a configuration source.  It is intended to help the user
// understand how the system acquired its configuration, and to aid in tracking
// down errors in configuration files.
type FileInfo struct {
	Path   string    // Path of configuration file.
	Parent *FileInfo // File that imported this configuration file, if any.
}

// ImportString creates a string describing the import hierarchy of the given
// FileInfo.  It should be called on the *parent* of the file of interest.
func (fi *FileInfo) ImportString() string {
	s := ""

	first := true
	for fi != nil {
		if !first {
			s += ", "
		}
		first = false

		s += fmt.Sprintf("imported from %s", fi.Path)
		fi = fi.Parent
	}

	return s
}

// ErrTree decorates the given error with a description of the configuration
// file's import hierarchy.  If a configuration error is encountered, this
// function should be called on the configuration file's *parent*.
func (fi *FileInfo) ErrTree(err error) error {
	if fi == nil {
		return err
	}

	return FmtNewtError("%s - %s", err.Error(), fi.ImportString())
}
