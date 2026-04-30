package memorypalace

import "time"

type SearchMode string
type SyncStatus string

const (
	SearchModeKeyword SearchMode = "keyword"
	SearchModeVector  SearchMode = "vector"
	SearchModeHybrid  SearchMode = "hybrid"

	SyncStatusActive     SyncStatus = "active"
	SyncStatusMissing    SyncStatus = "missing"
	SyncStatusError      SyncStatus = "error"
	SyncStatusTombstoned SyncStatus = "tombstoned"
)

type Source struct {
	ID          string
	Kind        string
	Scope       string
	Path        string
	Title       string
	ExternalRef string
	ContentHash string
	Metadata    map[string]string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Entry struct {
	ID            string
	SourceID      string
	Seq           int
	ParentEntryID string
	Kind          string
	Role          string
	Name          string
	Status        string
	Text          string
	PayloadJSON   string
	ContentHash   string
	EventAt       time.Time
	CreatedAt     time.Time
	Metadata      map[string]string
}

type Chunk struct {
	ID        string
	SourceID  string
	Ordinal   int
	Kind      string
	Wing      string
	Room      string
	Path      string
	Content   string
	StartSeq  int
	EndSeq    int
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ChunkEntry struct {
	ChunkID string
	EntryID string
	Ordinal int
}

type SourceSync struct {
	SourceID      string
	SyncKind      string
	ExternalRef   string
	LastSeenAt    time.Time
	LastScannedAt time.Time
	SourceVersion string
	ContentHash   string
	Status        SyncStatus
	LastError     string
	Metadata      map[string]string
}

type SyncSource struct {
	SourceID      string
	SyncKind      string
	ExternalRef   string
	Path          string
	Title         string
	Scope         string
	SourceKind    string
	SourceVersion string
	Content       string
	Metadata      map[string]string
	SeenAt        time.Time
}

type SyncResult struct {
	SourceID    string
	Action      string
	ChunkCount  int
	ContentHash string
	IndexError  string
}

type SearchFilter struct {
	SourceID        string
	Wing            string
	Room            string
	PathPrefix      string
	IncludeInactive bool
}

type SearchRequest struct {
	Query       string
	QueryVector []float32
	Limit       int
	Mode        SearchMode
	Filter      SearchFilter
}

type SearchHit struct {
	Chunk          Chunk
	Score          float64
	KeywordScore   float64
	VectorDistance float64
}

type IndexRecord struct {
	Chunk  Chunk
	Vector []float32
}

type Capabilities struct {
	Driver        string
	Keyword       bool
	Vector        bool
	SQLiteVersion string
	VectorVersion string
}
