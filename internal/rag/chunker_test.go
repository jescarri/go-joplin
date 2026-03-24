package rag

import (
	"strings"
	"testing"
)

func TestChunk_EmptyText(t *testing.T) {
	result := Chunk("", 512, 50)
	if len(result) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(result))
	}
}

func TestChunk_WhitespaceOnly(t *testing.T) {
	result := Chunk("   \t\n  ", 512, 50)
	if len(result) != 0 {
		t.Fatalf("expected 0 chunks, got %d", len(result))
	}
}

func TestChunk_ShortText(t *testing.T) {
	result := Chunk("hello world", 512, 50)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	if result[0].Content != "hello world" {
		t.Errorf("got %q, want %q", result[0].Content, "hello world")
	}
	if result[0].TokenCount != 2 {
		t.Errorf("token count: got %d, want 2", result[0].TokenCount)
	}
	if result[0].Index != 0 {
		t.Errorf("index: got %d, want 0", result[0].Index)
	}
}

func TestChunk_ExactChunkSize(t *testing.T) {
	tokens := make([]string, 10)
	for i := range tokens {
		tokens[i] = "word"
	}
	text := strings.Join(tokens, " ")

	result := Chunk(text, 10, 3)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk for exact size, got %d", len(result))
	}
	if result[0].TokenCount != 10 {
		t.Errorf("token count: got %d, want 10", result[0].TokenCount)
	}
}

func TestChunk_MultipleChunks(t *testing.T) {
	// 20 tokens, chunk size 10, overlap 3 → step=7
	// chunk 0: tokens[0:10], chunk 1: tokens[7:17], chunk 2: tokens[14:20]
	tokens := make([]string, 20)
	for i := range tokens {
		tokens[i] = "w"
	}
	text := strings.Join(tokens, " ")

	result := Chunk(text, 10, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(result))
	}
	if result[0].TokenCount != 10 {
		t.Errorf("chunk 0 tokens: got %d, want 10", result[0].TokenCount)
	}
	if result[1].TokenCount != 10 {
		t.Errorf("chunk 1 tokens: got %d, want 10", result[1].TokenCount)
	}
	// Last chunk may be smaller
	if result[2].TokenCount != 6 {
		t.Errorf("chunk 2 tokens: got %d, want 6", result[2].TokenCount)
	}
}

func TestChunk_OverlapContent(t *testing.T) {
	// Use distinct tokens so we can verify overlap
	text := "a b c d e f g h i j"
	// 10 tokens, chunk size 6, overlap 2 → step=4
	result := Chunk(text, 6, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(result))
	}
	// chunk 0: a b c d e f
	// chunk 1: e f g h i j
	if result[0].Content != "a b c d e f" {
		t.Errorf("chunk 0: got %q", result[0].Content)
	}
	if result[1].Content != "e f g h i j" {
		t.Errorf("chunk 1: got %q", result[1].Content)
	}
}

func TestChunk_Unicode(t *testing.T) {
	text := "日本語 テスト 文字列"
	result := Chunk(text, 512, 50)
	if len(result) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(result))
	}
	if result[0].Content != "日本語 テスト 文字列" {
		t.Errorf("got %q", result[0].Content)
	}
	if result[0].TokenCount != 3 {
		t.Errorf("token count: got %d, want 3", result[0].TokenCount)
	}
}

func TestChunk_IndexesSequential(t *testing.T) {
	tokens := make([]string, 100)
	for i := range tokens {
		tokens[i] = "w"
	}
	text := strings.Join(tokens, " ")
	result := Chunk(text, 10, 2)
	for i, c := range result {
		if c.Index != i {
			t.Errorf("chunk %d has index %d", i, c.Index)
		}
	}
}
