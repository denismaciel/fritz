//go:build !cgo

package sqlite

import "fritz/pkg/memorypalace"

func serializeVector([]float32) ([]byte, error) {
	return nil, memorypalace.ErrVectorUnsupported
}
