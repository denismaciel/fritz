package memorypalace

import (
	"fmt"
	"sort"
	"strings"
)

const (
	documentChunkKind     = "document_span"
	documentDeriveVersion = "v1"
	maxChunkChars         = 900
)

func deriveDocument(source Source, entries []Entry) ([]Chunk, []ChunkEntry, error) {
	if strings.TrimSpace(source.ID) == "" {
		return nil, nil, fmt.Errorf("%w: source id required", ErrInvalidRequest)
	}
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Seq == sorted[j].Seq {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].Seq < sorted[j].Seq
	})

	var chunks []Chunk
	var links []ChunkEntry
	for _, entry := range sorted {
		if strings.TrimSpace(entry.ID) == "" {
			return nil, nil, fmt.Errorf("%w: entry id required", ErrInvalidRequest)
		}
		if strings.TrimSpace(entry.Text) == "" {
			continue
		}
		parts := splitDocumentText(entry.Text, maxChunkChars)
		for idx, part := range parts {
			chunkID := stableID("chunk", source.ID, entry.ID, fmt.Sprintf("%d", idx), part)
			metadata := map[string]string{
				"derivation_version": documentDeriveVersion,
				"entry_id":           entry.ID,
				"entry_kind":         entry.Kind,
				"source_kind":        source.Kind,
			}
			if wing := firstNonEmpty(entry.Metadata["wing"], source.Metadata["wing"]); wing != "" {
				metadata["wing"] = wing
			}
			if room := firstNonEmpty(entry.Metadata["room"], source.Metadata["room"]); room != "" {
				metadata["room"] = room
			}
			chunks = append(chunks, Chunk{
				ID:       chunkID,
				SourceID: source.ID,
				Ordinal:  len(chunks),
				Kind:     documentChunkKind,
				Wing:     firstNonEmpty(entry.Metadata["wing"], source.Metadata["wing"], "documents"),
				Room:     firstNonEmpty(entry.Metadata["room"], source.Metadata["room"], "general"),
				Path:     source.Path,
				Content:  part,
				StartSeq: entry.Seq,
				EndSeq:   entry.Seq,
				Metadata: metadata,
			})
			links = append(links, ChunkEntry{
				ChunkID: chunkID,
				EntryID: entry.ID,
				Ordinal: 0,
			})
		}
	}
	return chunks, links, nil
}

func splitDocumentText(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= limit {
		return []string{text}
	}
	paragraphs := splitParagraphs(text)
	if len(paragraphs) == 0 {
		return []string{text}
	}
	var out []string
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, strings.Join(current, "\n\n"))
		current = nil
		currentLen = 0
	}
	for _, para := range paragraphs {
		if len(para) > limit {
			flush()
			out = append(out, splitLongParagraph(para, limit)...)
			continue
		}
		nextLen := currentLen + len(para)
		if len(current) > 0 {
			nextLen += 2
		}
		if nextLen > limit {
			flush()
		}
		current = append(current, para)
		currentLen += len(para)
		if len(current) > 1 {
			currentLen += 2
		}
	}
	flush()
	if len(out) == 0 {
		return []string{text}
	}
	return out
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	out := make([]string, 0, len(raw))
	for _, para := range raw {
		para = strings.TrimSpace(para)
		if para != "" {
			out = append(out, para)
		}
	}
	return out
}

func splitLongParagraph(text string, limit int) []string {
	var out []string
	runes := []rune(text)
	for start := 0; start < len(runes); start += limit {
		end := start + limit
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, strings.TrimSpace(string(runes[start:end])))
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
