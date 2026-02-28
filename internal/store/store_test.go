package store

import (
	"os"
	"testing"
)

// testDB creates a temporary database for testing and returns it along with a cleanup function.
func testDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(dir)
	})
	return db
}
