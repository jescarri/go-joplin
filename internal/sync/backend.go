package sync

// SyncBackend abstracts the remote sync target (Joplin Server HTTP API or S3).
// The engine uses Get/Put/Delete with Joplin Server-style paths; S3 backend
// translates these to S3 operations.
type SyncBackend interface {
	Authenticate() error
	IsAuthenticated() bool
	AcquireLock() (interface{}, error)
	ReleaseLock(lock interface{}) error
	Get(path string) ([]byte, error)
	Put(path string, content []byte) error
	Delete(path string) error
	// SyncTarget returns the sync target id (8 = S3, 9 = Joplin Server).
	SyncTarget() int
}
