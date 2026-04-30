// Package memorypalace provides a small, promotable memory/retrieval core.
//
// Canonical sources and entries are the source of truth. Retrieval chunks,
// FTS rows, and vector rows are derived state that can be rebuilt.
//
// The canonical store, embedder, and retrieval index stay behind narrow
// interfaces so the backend mix can change without rewriting app semantics.
package memorypalace
