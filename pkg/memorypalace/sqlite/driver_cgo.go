//go:build cgo

package sqlite

import (
	_ "github.com/mattn/go-sqlite3"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
)

func ensureDriver() error {
	sqlitevec.Auto()
	return nil
}
