package memorypalace

import "errors"

var (
	ErrInvalidRequest    = errors.New("memorypalace: invalid request")
	ErrNotFound          = errors.New("memorypalace: not found")
	ErrMissingEmbedder   = errors.New("memorypalace: missing embedder")
	ErrVectorUnsupported = errors.New("memorypalace: vector search unsupported")
)
