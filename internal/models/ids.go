package models

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID generates a 32-character lowercase hex ID using crypto/rand,
// matching Joplin's ID format.
func NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}
