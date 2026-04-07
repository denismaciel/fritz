package app

import "fmt"

type Error struct {
	Kind string
	Err  error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s error: %v", e.Kind, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func wrapError(kind string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Err: err}
}
