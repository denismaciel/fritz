package memorypalace

import (
	"strings"
	"testing"
)

func TestDeriveDocumentStableAndChunked(t *testing.T) {
	source := Source{
		ID:   "src-1",
		Kind: "document",
		Path: "docs/note.md",
		Metadata: map[string]string{
			"wing": "filesystem",
		},
	}
	entry := Entry{
		ID:       "entry-1",
		SourceID: "src-1",
		Seq:      0,
		Kind:     "document_body",
		Text:     "first paragraph\n\n" + strings.Repeat("long ", 300),
	}

	chunksA, linksA, err := deriveDocument(source, []Entry{entry})
	if err != nil {
		t.Fatalf("deriveDocument() error = %v", err)
	}
	chunksB, linksB, err := deriveDocument(source, []Entry{entry})
	if err != nil {
		t.Fatalf("deriveDocument() second error = %v", err)
	}
	if len(chunksA) < 2 {
		t.Fatalf("chunks = %#v", chunksA)
	}
	if len(chunksA) != len(chunksB) || len(linksA) != len(linksB) {
		t.Fatalf("non-deterministic lengths: %d %d / %d %d", len(chunksA), len(chunksB), len(linksA), len(linksB))
	}
	for i := range chunksA {
		if chunksA[i].ID != chunksB[i].ID {
			t.Fatalf("chunk ids differ: %q != %q", chunksA[i].ID, chunksB[i].ID)
		}
		if chunksA[i].Kind != "document_span" {
			t.Fatalf("chunk kind = %#v", chunksA[i])
		}
	}
}
