//go:build !cgo

package sqlite

import (
	"fmt"

	"fritz/pkg/memorypalace"
)

func ensureDriver() error {
	return fmt.Errorf("%w: sqlite adapter requires cgo", memorypalace.ErrVectorUnsupported)
}
