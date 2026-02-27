package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/jescarri/go-joplin/internal/config"
	"github.com/jescarri/go-joplin/internal/s3"
)

// S3Backend implements SyncBackend by translating Joplin Server-style API paths
// to S3 operations. Compatible with AWS S3 and S3-compatible storage (e.g. MinIO).
type S3Backend struct {
	client *s3.Client
	target int
}

// NewS3Backend creates an S3 sync backend. Credentials come from env vars (if set) or
// sync.8.username and sync.8.password in the Joplin config, matching Joplin Server config.
func NewS3Backend(cfg *config.Config) (*S3Backend, error) {
	ctx := context.Background()
	client, err := s3.NewClient(ctx, s3.Config{
		Bucket:          cfg.S3Bucket,
		Region:          cfg.S3Region,
		Endpoint:        cfg.S3URL,
		ForcePathStyle:  cfg.S3ForcePathStyle,
		AccessKeyID:     cfg.S3AccessKey,
		SecretAccessKey: cfg.S3SecretKey,
	})
	if err != nil {
		return nil, err
	}
	return &S3Backend{client: client, target: 8}, nil
}

// Authenticate is a no-op for S3 (credentials from env).
func (b *S3Backend) Authenticate() error { return nil }

// IsAuthenticated is always true for S3 once the client is created.
func (b *S3Backend) IsAuthenticated() bool { return true }

// AcquireLock is a no-op for S3 (no server-side lock API).
func (b *S3Backend) AcquireLock() (interface{}, error) { return nil, nil }

// ReleaseLock is a no-op for S3.
func (b *S3Backend) ReleaseLock(interface{}) error { return nil }

// SyncTarget returns 8 (S3).
func (b *S3Backend) SyncTarget() int { return b.target }

// Get implements SyncBackend by parsing path and calling S3 GetObject or listing.
func (b *S3Backend) Get(path string) ([]byte, error) {
	path, rawQuery, _ := strings.Cut(path, "?")
	key, op := b.parsePath(path)
	if key == "" && op == "delta" {
		return b.getDelta(rawQuery)
	}
	if key == "" && op == "children" {
		return b.getChildren(rawQuery)
	}
	if key != "" && op == "content" {
		key = b.decodeKey(key)
		return b.client.GetObject(context.Background(), key)
	}
	return nil, fmt.Errorf("unsupported S3 path: %s", path)
}

// Put implements SyncBackend by parsing path and calling S3 PutObject.
func (b *S3Backend) Put(path string, content []byte) error {
	key, op := b.parsePath(path)
	if key == "" || op != "content" {
		return fmt.Errorf("unsupported S3 put path: %s", path)
	}
	key = b.decodeKey(key)
	return b.client.PutObject(context.Background(), key, content)
}

// Delete implements SyncBackend by parsing path and calling S3 DeleteObject.
func (b *S3Backend) Delete(path string) error {
	key, op := b.parsePath(path)
	if key == "" || op != "content" {
		return fmt.Errorf("unsupported S3 delete path: %s", path)
	}
	key = b.decodeKey(key)
	return b.client.DeleteObject(context.Background(), key)
}

// decodeKey reverses url.PathEscape so the S3 key matches the key in the bucket.
func (b *S3Backend) decodeKey(key string) string {
	u, err := url.PathUnescape(key)
	if err != nil {
		return key
	}
	return u
}

// parsePath extracts key and operation from paths like:
//   - /api/items/root:/:/delta -> ("", "delta")
//   - /api/items/root:/:/children -> ("", "children")
//   - /api/items/root:/info.json:/content -> ("info.json", "content")
//   - /api/items/root:/abc123.md:/content -> ("abc123.md", "content")
func (b *S3Backend) parsePath(path string) (key, op string) {
	const prefix = "/api/items/root:/"
	if !strings.HasPrefix(path, prefix) {
		return "", ""
	}
	rest := path[len(prefix):]
	parts := strings.SplitN(rest, ":/", 2)
	if len(parts) < 2 {
		return "", ""
	}
	key = strings.TrimSuffix(parts[0], ":")
	op = parts[1]
	if op == "content" || strings.HasSuffix(op, ":/content") {
		return parts[0], "content"
	}
	if key == "" || key == ":" {
		key = ""
		if op == "delta" || strings.HasSuffix(rest, "delta") {
			return "", "delta"
		}
		if op == "children" || strings.HasSuffix(rest, "children") {
			return "", "children"
		}
	}
	return "", ""
}

func (b *S3Backend) getDelta(rawQuery string) ([]byte, error) {
	var cursor string
	if rawQuery != "" {
		vals, _ := url.ParseQuery(rawQuery)
		cursor = vals.Get("cursor")
	}
	keys, updatedTimes, nextToken, err := b.client.ListObjects(context.Background(), "", cursor, 1000)
	if err != nil {
		return nil, err
	}
	items := make([]DeltaItem, 0, len(keys))
	for i, k := range keys {
		ts := int64(0)
		if i < len(updatedTimes) {
			ts = updatedTimes[i]
		}
		items = append(items, DeltaItem{
			ItemName:    k,
			Type:        1, // put
			UpdatedTime: ts,
		})
	}
	resp := DeltaResponse{
		Items:   items,
		Cursor:  nextToken,
		HasMore: nextToken != "",
	}
	return json.Marshal(resp)
}

func (b *S3Backend) getChildren(rawQuery string) ([]byte, error) {
	var cursor string
	if rawQuery != "" {
		vals, _ := url.ParseQuery(rawQuery)
		cursor = vals.Get("cursor")
	}
	keys, _, nextToken, err := b.client.ListObjects(context.Background(), "", cursor, 1000)
	if err != nil {
		return nil, err
	}
	items := make([]ChildItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, ChildItem{Name: k})
	}
	resp := ChildrenResponse{
		Items:   items,
		Cursor:  nextToken,
		HasMore: nextToken != "",
	}
	return json.Marshal(resp)
}
