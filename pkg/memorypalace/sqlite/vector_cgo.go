//go:build cgo

package sqlite

import sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"

func serializeVector(vector []float32) ([]byte, error) {
	return sqlitevec.SerializeFloat32(vector)
}
