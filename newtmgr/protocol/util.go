package protocol

import (
	// "bytes"
	"runtime"
)




type NewtError struct {
	Parent     error
	Text       string
	StackTrace []byte
}


func (se *NewtError) Error() string {
	return se.Text
}

func NewNewtError(msg string) *NewtError {
	err := &NewtError{
		Text:       msg,
		StackTrace: make([]byte, 65536),
	}

	stackLen := runtime.Stack(err.StackTrace, true)
	err.StackTrace = err.StackTrace[:stackLen]

	return err
}