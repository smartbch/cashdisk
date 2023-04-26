package types

import "errors"

var (
	ErrReadOnly = errors.New("the shared directory is readonly")
)
