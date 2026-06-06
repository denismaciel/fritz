package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/prompt"
)

const CurrentSessionVersion = 1

const maxSessionLineBytes = 8 * 1024 * 1024

type EntryType string

const (
	SessionEntryType             EntryType = "session"
	MessageEntryType             EntryType = "message"
	ModelChangeEntryType         EntryType = "model_change"
	ThinkingLevelChangeEntryType EntryType = "thinking_level_change"
	CompactionEntryType          EntryType = "compaction"
	BranchSummaryEntryType       EntryType = "branch_summary"
	SessionInfoEntryType         EntryType = "session_info"
)

type MessageKind string

const (
	UserPromptKind    MessageKind = "user_prompt"
	ModelResponseKind MessageKind = "model_response"
	ToolResultKind    MessageKind = "tool_result"
)

type Line struct {
	Type EntryType `json:"type"`

	Version       int     `json:"version,omitempty"`
	ID            string  `json:"id"`
	ParentID      *string `json:"parentId,omitempty"`
	Timestamp     string  `json:"timestamp"`
	Cwd           string  `json:"cwd,omitempty"`
	ParentSession string  `json:"parentSession,omitempty"`

	Kind    MessageKind   `json:"kind,omitempty"`
	Message model.Message `json:"message,omitempty"`
	Text    string        `json:"text,omitempty"`

	Provider      string `json:"provider,omitempty"`
	ModelID       string `json:"modelId,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`

	Summary             string          `json:"summary,omitempty"`
	FirstKeptEntryID    string          `json:"firstKeptEntryId,omitempty"`
	MessagesBefore      int             `json:"messagesBefore,omitempty"`
	ReplacementMessages []model.Message `json:"replacementMessages,omitempty"`
	FromID              string          `json:"fromId,omitempty"`
	Details             json.RawMessage `json:"details,omitempty"`

	Name string `json:"name,omitempty"`
}

func (l Line) IsHeader() bool {
	return l.Type == SessionEntryType && l.Cwd != ""
}

func (l Line) HasParent() bool {
	return l.ParentID != nil
}

type SessionInfo struct {
	Path            string
	ID              string
	Cwd             string
	Name            string
	ParentSession   string
	Created         time.Time
	Modified        time.Time
	MessageCount    int
	FirstMessage    string
	AllMessagesText string
}

type Context struct {
	Messages      []model.Message
	Transcript    chat.Transcript
	ModelID       string
	ThinkingLevel string
}

type TreeNode struct {
	Entry    Line
	Children []TreeNode
}

type Manager struct {
	sessionID   string
	sessionFile string
	sessionDir  string
	cwd         string
	persist     bool
	flushed     bool
	lines       []Line
	byID        map[string]Line
	leafID      string
}

func Create(cwd string, sessionRoot string) (*Manager, error) {
	dir, err := resolveSessionDir(cwd, sessionRoot)
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		sessionDir: dir,
		cwd:        cwd,
		persist:    true,
		byID:       map[string]Line{},
	}
	if err := manager.newSession(""); err != nil {
		return nil, err
	}
	return manager, nil
}

func ContinueRecent(cwd string, sessionRoot string) (*Manager, error) {
	dir, err := resolveSessionDir(cwd, sessionRoot)
	if err != nil {
		return nil, err
	}
	path, _ := mostRecentSessionFile(dir)
	if path == "" {
		return Create(cwd, sessionRoot)
	}
	return Open(path, sessionRoot)
}

func Open(path string, sessionRoot string) (*Manager, error) {
	resolved := filepath.Clean(path)
	lines, err := loadLinesFromFile(resolved)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 || !lines[0].IsHeader() {
		return nil, fmt.Errorf("invalid session file %q", resolved)
	}
	lines = migrateLines(lines)
	header := lines[0]
	dir := filepath.Dir(resolved)
	if sessionRoot != "" {
		dir, err = resolveSessionDir(header.Cwd, sessionRoot)
		if err != nil {
			return nil, err
		}
	}
	manager := &Manager{
		sessionID:   header.ID,
		sessionFile: resolved,
		sessionDir:  dir,
		cwd:         header.Cwd,
		persist:     true,
		flushed:     true,
		lines:       lines,
		byID:        map[string]Line{},
	}
	manager.rebuildIndex()
	return manager, nil
}

func InMemory(cwd string) *Manager {
	manager := &Manager{
		cwd:     cwd,
		persist: false,
		byID:    map[string]Line{},
	}
	_ = manager.newSession("")
	return manager
}

func ForkFrom(sourcePath string, targetCwd string, sessionRoot string) (*Manager, error) {
	sourceLines, err := loadLinesFromFile(sourcePath)
	if err != nil {
		return nil, err
	}
	if len(sourceLines) == 0 || !sourceLines[0].IsHeader() {
		return nil, fmt.Errorf("invalid source session %q", sourcePath)
	}
	dir, err := resolveSessionDir(targetCwd, sessionRoot)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	sessionID := newID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	target := filepath.Join(dir, fmt.Sprintf("%s_%s.jsonl", fileTimestamp(now), sessionID))
	header := Line{
		Type:          SessionEntryType,
		Version:       CurrentSessionVersion,
		ID:            sessionID,
		Timestamp:     now,
		Cwd:           targetCwd,
		ParentSession: filepath.Clean(sourcePath),
	}
	lines := []Line{header}
	for _, line := range sourceLines[1:] {
		lines = append(lines, line)
	}
	if err := rewriteLines(target, lines); err != nil {
		return nil, err
	}
	return Open(target, sessionRoot)
}

func List(cwd string, sessionRoot string) ([]SessionInfo, error) {
	dir, err := resolveSessionDir(cwd, sessionRoot)
	if err != nil {
		return nil, err
	}
	return listSessionsFromDir(dir)
}

func ListAll(cwd string, sessionRoot string) ([]SessionInfo, error) {
	base := config.WorkspacesStateRoot()
	if strings.TrimSpace(sessionRoot) != "" {
		dir, err := resolveSessionDir(cwd, sessionRoot)
		if err != nil {
			return nil, err
		}
		base = filepath.Dir(dir)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		infos, err := listSessionsFromDir(filepath.Join(base, entry.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, infos...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Modified.After(out[j].Modified)
	})
	return out, nil
}

func (m *Manager) newSession(parentSession string) error {
	m.sessionID = newUUIDLike()
	m.leafID = ""
	m.flushed = false
	m.lines = []Line{{
		Type:          SessionEntryType,
		Version:       CurrentSessionVersion,
		ID:            m.sessionID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Cwd:           m.cwd,
		ParentSession: parentSession,
	}}
	m.byID = map[string]Line{}
	if !m.persist {
		return nil
	}
	if err := os.MkdirAll(m.sessionDir, 0o755); err != nil {
		return err
	}
	m.sessionFile = filepath.Join(m.sessionDir, fmt.Sprintf("%s_%s.jsonl", fileTimestamp(m.lines[0].Timestamp), m.sessionID))
	return nil
}

func (m *Manager) SetSessionFile(path string) error {
	opened, err := Open(path, m.sessionDir)
	if err != nil {
		return err
	}
	*m = *opened
	return nil
}

func (m *Manager) rebuildIndex() {
	m.byID = map[string]Line{}
	m.leafID = ""
	for _, line := range m.lines[1:] {
		m.byID[line.ID] = line
		m.leafID = line.ID
	}
}

func (m *Manager) appendLine(line Line) error {
	m.lines = append(m.lines, line)
	m.byID[line.ID] = line
	m.leafID = line.ID
	return m.persistLine(line)
}

func (m *Manager) persistLine(line Line) error {
	if !m.persist || m.sessionFile == "" {
		return nil
	}
	hasModel := false
	for _, existing := range m.lines[1:] {
		if existing.Type == MessageEntryType && existing.Kind == ModelResponseKind {
			hasModel = true
			break
		}
	}
	if !hasModel {
		m.flushed = false
		return nil
	}
	if !m.flushed {
		if err := rewriteLines(m.sessionFile, m.lines); err != nil {
			return err
		}
		m.flushed = true
		return nil
	}
	file, err := os.OpenFile(m.sessionFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	_, err = file.WriteString(string(data) + "\n")
	return err
}

func (m *Manager) AppendPrompt(prompt string) (Line, error) {
	line := Line{
		Type:      MessageEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      UserPromptKind,
		Message:   model.TextMessage(model.UserRole, prompt),
		Text:      prompt,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendModelResponse(response model.Response) (Line, error) {
	line := Line{
		Type:      MessageEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      ModelResponseKind,
		Message:   response.Message,
		Text:      response.Text,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendToolResult(resultText string, resultMessage model.Message) (Line, error) {
	line := Line{
		Type:      MessageEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Kind:      ToolResultKind,
		Message:   resultMessage,
		Text:      resultText,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendModelChange(modelID string) (Line, error) {
	line := Line{
		Type:      ModelChangeEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		ModelID:   modelID,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendThinkingLevelChange(level string) (Line, error) {
	line := Line{
		Type:          ThinkingLevelChangeEntryType,
		ID:            newID(),
		ParentID:      maybeString(m.leafID),
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		ThinkingLevel: level,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendCompaction(summary string, firstKeptEntryID string, messagesBefore int, replacementMessages []model.Message) (Line, error) {
	line := Line{
		Type:                CompactionEntryType,
		ID:                  newID(),
		ParentID:            maybeString(m.leafID),
		Timestamp:           time.Now().UTC().Format(time.RFC3339Nano),
		Summary:             summary,
		FirstKeptEntryID:    firstKeptEntryID,
		MessagesBefore:      messagesBefore,
		ReplacementMessages: append([]model.Message(nil), replacementMessages...),
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendBranchSummary(fromID string, summary string) (Line, error) {
	line := Line{
		Type:      BranchSummaryEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		FromID:    fromID,
		Summary:   summary,
	}
	return line, m.appendLine(line)
}

func (m *Manager) AppendSessionInfo(name string) (Line, error) {
	line := Line{
		Type:      SessionInfoEntryType,
		ID:        newID(),
		ParentID:  maybeString(m.leafID),
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Name:      strings.TrimSpace(name),
	}
	return line, m.appendLine(line)
}

func (m *Manager) GetSessionName() string {
	for i := len(m.lines) - 1; i >= 1; i-- {
		if m.lines[i].Type == SessionInfoEntryType {
			return strings.TrimSpace(m.lines[i].Name)
		}
	}
	return ""
}

func (m *Manager) SessionID() string {
	return m.sessionID
}

func (m *Manager) SessionFile() string {
	return m.sessionFile
}

func (m *Manager) SessionDir() string {
	return m.sessionDir
}

func (m *Manager) Cwd() string {
	return m.cwd
}

func (m *Manager) IsPersisted() bool {
	return m.persist
}

func (m *Manager) LeafID() string {
	return m.leafID
}

func (m *Manager) Header() Line {
	return m.lines[0]
}

func (m *Manager) Entries() []Line {
	return append([]Line(nil), m.lines[1:]...)
}

func (m *Manager) Entry(id string) (Line, bool) {
	line, ok := m.byID[id]
	return line, ok
}

func (m *Manager) Branch(fromID string) []Line {
	currentID := fromID
	if currentID == "" {
		currentID = m.leafID
	}
	var out []Line
	for currentID != "" {
		line, ok := m.byID[currentID]
		if !ok {
			break
		}
		out = append(out, line)
		if line.ParentID == nil {
			break
		}
		currentID = *line.ParentID
	}
	reverseLines(out)
	return out
}

func (m *Manager) BuildContext() Context {
	return BuildContext(m.Branch(""))
}

func (m *Manager) Tree() []TreeNode {
	nodes := map[string]*TreeNode{}
	var roots []TreeNode
	for _, line := range m.lines[1:] {
		line := line
		nodes[line.ID] = &TreeNode{Entry: line}
	}
	for _, line := range m.lines[1:] {
		node := nodes[line.ID]
		if line.ParentID == nil {
			roots = append(roots, *node)
			continue
		}
		parent := nodes[*line.ParentID]
		if parent == nil {
			roots = append(roots, *node)
			continue
		}
		parent.Children = append(parent.Children, *node)
	}
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].Entry.Timestamp < roots[j].Entry.Timestamp
	})
	return roots
}

func (m *Manager) MoveLeaf(id string) error {
	if id == "" {
		m.leafID = ""
		return nil
	}
	if _, ok := m.byID[id]; !ok {
		return fmt.Errorf("entry %q not found", id)
	}
	m.leafID = id
	return nil
}

func (m *Manager) BranchWithSummary(targetID string, summary string) (Line, error) {
	if err := m.MoveLeaf(targetID); err != nil {
		return Line{}, err
	}
	return m.AppendBranchSummary(targetID, summary)
}

func (m *Manager) CreateBranchedSession(leafID string) (string, error) {
	if !m.persist {
		return "", errors.New("session is not persisted")
	}
	branch := m.Branch(leafID)
	if len(branch) == 0 {
		return "", fmt.Errorf("entry %q not found", leafID)
	}
	manager, err := Create(m.cwd, m.sessionDir)
	if err != nil {
		return "", err
	}
	header := manager.lines[0]
	header.ParentSession = m.sessionFile
	manager.lines[0] = header
	manager.lines = append([]Line{header}, branch...)
	manager.rebuildIndex()
	manager.flushed = false
	if err := rewriteLines(manager.sessionFile, manager.lines); err != nil {
		return "", err
	}
	manager.flushed = true
	return manager.sessionFile, nil
}

func (m *Manager) Stats() SessionInfo {
	context := m.BuildContext()
	info := SessionInfo{
		Path:          m.sessionFile,
		ID:            m.sessionID,
		Cwd:           m.cwd,
		Name:          m.GetSessionName(),
		ParentSession: m.lines[0].ParentSession,
		MessageCount:  len(context.Messages),
	}
	if t, err := time.Parse(time.RFC3339Nano, m.lines[0].Timestamp); err == nil {
		info.Created = t
	}
	if len(m.lines) > 0 {
		if t, err := time.Parse(time.RFC3339Nano, m.lines[len(m.lines)-1].Timestamp); err == nil {
			info.Modified = t
		}
	}
	for _, line := range m.lines[1:] {
		if line.Type != MessageEntryType {
			continue
		}
		if info.FirstMessage == "" && line.Text != "" {
			info.FirstMessage = line.Text
		}
		if line.Text != "" {
			if info.AllMessagesText != "" {
				info.AllMessagesText += "\n"
			}
			info.AllMessagesText += line.Text
		}
	}
	return info
}

func BuildContext(branch []Line) Context {
	var (
		messages []model.Message
		start    int
		legacy   *Line
	)
	for i, line := range branch {
		if line.Type != CompactionEntryType {
			continue
		}
		if len(line.ReplacementMessages) > 0 {
			messages = append([]model.Message(nil), line.ReplacementMessages...)
			start = i + 1
			legacy = nil
			continue
		}
		copyLine := line
		legacy = &copyLine
	}
	if legacy != nil && len(messages) == 0 {
		if legacy.Summary != "" {
			messages = append(messages, model.TextMessage(model.UserRole, prompt.BuildCompactionSummaryMessage(legacy.Summary)))
		}
		if legacy.FirstKeptEntryID != "" {
			for i, line := range branch {
				if line.ID == legacy.FirstKeptEntryID {
					start = i
					break
				}
			}
		}
	}
	var modelID string
	var thinkingLevel string
	for _, line := range branch[start:] {
		switch line.Type {
		case MessageEntryType:
			messages = append(messages, line.Message)
		case BranchSummaryEntryType:
			messages = append(messages, model.TextMessage(model.UserRole, "Branch summary:\n"+line.Summary))
		case ModelChangeEntryType:
			modelID = line.ModelID
		case ThinkingLevelChangeEntryType:
			thinkingLevel = line.ThinkingLevel
		}
	}
	return Context{
		Messages:      messages,
		Transcript:    transcriptFromMessages(messages),
		ModelID:       modelID,
		ThinkingLevel: thinkingLevel,
	}
}

func transcriptFromMessages(messages []model.Message) chat.Transcript {
	var transcript chat.Transcript
	pendingUser := ""
	for _, msg := range messages {
		switch msg.Role {
		case model.UserRole:
			text := strings.TrimSpace(msg.Text())
			if text == "" {
				continue
			}
			if prompt.IsCompactionSummaryMessage(text) || strings.HasPrefix(text, "Branch summary:\n") {
				pendingUser = ""
				continue
			}
			pendingUser = text
		case model.ModelRole:
			text := strings.TrimSpace(msg.Text())
			if text == "" || pendingUser == "" {
				continue
			}
			transcript = append(transcript, chat.Turn{User: pendingUser, Assistant: text})
			pendingUser = ""
		}
	}
	return transcript
}

func resolveSessionDir(cwd string, sessionRoot string) (string, error) {
	return config.ResolveSessionDir(cwd, sessionRoot)
}

func encodePath(cwd string) string {
	return config.WorkspaceKey(cwd)
}

func maybeString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func newID() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func newUUIDLike() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func fileTimestamp(ts string) string {
	return strings.NewReplacer(":", "-", ".", "-").Replace(ts)
}

func loadLinesFromFile(path string) ([]Line, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []Line
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSessionLineBytes)
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var line Line
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("load session lines from %s: %w", path, err)
	}
	return lines, nil
}

func rewriteLines(path string, lines []Line) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	for _, line := range lines {
		if err := enc.Encode(line); err != nil {
			return err
		}
	}
	return nil
}

func migrateLines(lines []Line) []Line {
	if len(lines) == 0 {
		return lines
	}
	if lines[0].Type == SessionEntryType && lines[0].Version == 0 {
		lines[0].Version = CurrentSessionVersion
	}
	return lines
}

func listSessionsFromDir(dir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []SessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		manager, err := Open(filepath.Join(dir, entry.Name()), "")
		if err != nil {
			return nil, err
		}
		out = append(out, manager.Stats())
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Modified.After(out[j].Modified)
	})
	return out, nil
}

func mostRecentSessionFile(dir string) (string, error) {
	sessions, err := listSessionsFromDir(dir)
	if err != nil || len(sessions) == 0 {
		return "", err
	}
	return sessions[0].Path, nil
}

func reverseLines(lines []Line) {
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
}
