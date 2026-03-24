package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// VectorResult holds a single KNN search result.
type VectorResult struct {
	ChunkID  int64
	NoteID   string
	Distance float64
}

// GetNoteHash returns the stored body hash for a note, or "" if not found.
func (db *DB) GetNoteHash(noteID string) (string, error) {
	var hash string
	err := db.QueryRow("SELECT body_hash FROM rag_note_hashes WHERE note_id = ?", noteID).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

// UpsertNoteHash inserts or updates the body hash for a note.
func (db *DB) UpsertNoteHash(noteID, hash string) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO rag_note_hashes (note_id, body_hash, updated_at) VALUES (?, ?, ?)`,
		noteID, hash, time.Now().UnixMilli(),
	)
	return err
}

// DeleteChunksByNoteID removes all chunks and their vectors for a note.
func (db *DB) DeleteChunksByNoteID(noteID string) error {
	// Delete vectors first (references rag_chunks.id)
	if _, err := db.Exec("DELETE FROM rag_vec WHERE chunk_id IN (SELECT id FROM rag_chunks WHERE note_id = ?)", noteID); err != nil {
		return fmt.Errorf("delete vectors: %w", err)
	}
	if _, err := db.Exec("DELETE FROM rag_chunks WHERE note_id = ?", noteID); err != nil {
		return fmt.Errorf("delete chunks: %w", err)
	}
	return nil
}

// InsertChunk inserts a chunk and returns its auto-generated ID.
func (db *DB) InsertChunk(noteID string, idx int, content string, tokenCount int) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO rag_chunks (note_id, chunk_index, content, token_count) VALUES (?, ?, ?, ?)`,
		noteID, idx, content, tokenCount,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertChunkEmbedding inserts a vector into the rag_vec virtual table.
func (db *DB) InsertChunkEmbedding(chunkID int64, embedding []float32) error {
	blob, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	_, err = db.Exec("INSERT INTO rag_vec (chunk_id, embedding) VALUES (?, ?)", chunkID, string(blob))
	return err
}

// SearchVectors performs a KNN search and returns chunk IDs with distances.
func (db *DB) SearchVectors(embedding []float32, limit int) ([]VectorResult, error) {
	blob, err := json.Marshal(embedding)
	if err != nil {
		return nil, fmt.Errorf("marshal query embedding: %w", err)
	}

	rows, err := db.Query(`
		SELECT cv.chunk_id, c.note_id, cv.distance
		FROM rag_vec cv
		JOIN rag_chunks c ON c.id = cv.chunk_id
		WHERE cv.embedding MATCH ? AND cv.k = ?
		ORDER BY cv.distance`, string(blob), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []VectorResult
	for rows.Next() {
		var r VectorResult
		if err := rows.Scan(&r.ChunkID, &r.NoteID, &r.Distance); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// DeleteNoteRAGData removes all RAG data (vectors, chunks, hash) for a note.
func (db *DB) DeleteNoteRAGData(noteID string) error {
	if err := db.DeleteChunksByNoteID(noteID); err != nil {
		return err
	}
	_, err := db.Exec("DELETE FROM rag_note_hashes WHERE note_id = ?", noteID)
	return err
}

// ListAllNoteIDs returns all note IDs in the database.
func (db *DB) ListAllNoteIDs() ([]string, error) {
	rows, err := db.Query("SELECT id FROM notes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListIndexedNoteIDs returns note IDs that have RAG hashes.
func (db *DB) ListIndexedNoteIDs() ([]string, error) {
	rows, err := db.Query("SELECT note_id FROM rag_note_hashes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
