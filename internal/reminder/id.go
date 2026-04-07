package reminder

import (
	"crypto/rand"
	"encoding/hex"
)

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "reminder"
	}
	return hex.EncodeToString(buf[:])
}
