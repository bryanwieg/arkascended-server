package ark

import "errors"

var (
	ErrNotFound = errors.New("not found") // container, image, or volume does not exist
	ErrConflict = errors.New("conflict")  // volume in use, container already running
	ErrTimeout  = errors.New("timeout")   // poll/wait exceeded deadline
)
