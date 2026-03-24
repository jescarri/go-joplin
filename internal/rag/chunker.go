package rag

import "strings"

// ChunkResult holds a single chunk with its metadata.
type ChunkResult struct {
	Index      int
	Content    string
	TokenCount int
}

// Chunk splits text into fixed-size overlapping chunks measured in whitespace-delimited tokens.
// chunkSize is the max tokens per chunk; overlap is the number of tokens shared between adjacent chunks.
func Chunk(text string, chunkSize, overlap int) []ChunkResult {
	if text == "" {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 512
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return nil
	}

	if len(tokens) <= chunkSize {
		return []ChunkResult{{
			Index:      0,
			Content:    strings.Join(tokens, " "),
			TokenCount: len(tokens),
		}}
	}

	step := chunkSize - overlap
	if step <= 0 {
		step = 1
	}

	var chunks []ChunkResult
	for i := 0; i < len(tokens); i += step {
		end := i + chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}
		chunkTokens := tokens[i:end]
		chunks = append(chunks, ChunkResult{
			Index:      len(chunks),
			Content:    strings.Join(chunkTokens, " "),
			TokenCount: len(chunkTokens),
		})
		if end == len(tokens) {
			break
		}
	}
	return chunks
}
