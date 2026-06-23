package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fritz/internal/chat"
	"fritz/internal/config"
	"fritz/internal/model"
	"fritz/internal/prompt"
)

type CompactionPreparation struct {
	FirstKeptEntryID string
	DroppedTurns     chat.Transcript
	KeptTurns        chat.Transcript
	MessagesBefore   int
}

type compactOptions struct {
	keepTurns          int
	targetTokens       int
	customInstructions string
}

const compactionSystemPrompt = "You are a precise context compaction assistant. Summarize prior conversation turns for another model so it can continue the same task without losing important decisions, constraints, tool results, file paths, or unresolved work. Return only the compacted summary."

func PrepareCompaction(manager *Manager, keepTurns int) (CompactionPreparation, bool) {
	context := manager.BuildContext()
	if keepTurns <= 0 || len(context.Transcript) <= keepTurns {
		return CompactionPreparation{}, false
	}
	startIndex := len(context.Transcript) - keepTurns
	turnStartIDs := turnStartEntryIDs(manager.Branch(""))
	if len(turnStartIDs) <= keepTurns || startIndex >= len(turnStartIDs) {
		return CompactionPreparation{}, false
	}
	firstKept := turnStartIDs[startIndex]
	branch := manager.Branch("")
	messagesBefore := 0
	for _, line := range branch {
		if line.ID == firstKept {
			break
		}
		if line.Type == MessageEntryType {
			messagesBefore++
		}
	}
	return CompactionPreparation{
		FirstKeptEntryID: firstKept,
		DroppedTurns:     append(chat.Transcript(nil), context.Transcript[:startIndex]...),
		KeptTurns:        append(chat.Transcript(nil), context.Transcript[startIndex:]...),
		MessagesBefore:   messagesBefore,
	}, true
}

func Compact(ctx context.Context, manager *Manager, gateway model.Client, modelID string, keepTurns int, customInstructions string) (CompactionPreparation, string, error) {
	return compactWithOptions(ctx, manager, gateway, modelID, compactOptions{
		keepTurns:          keepTurns,
		customInstructions: customInstructions,
	})
}

func compactWithOptions(ctx context.Context, manager *Manager, gateway model.Client, modelID string, opts compactOptions) (CompactionPreparation, string, error) {
	preparation, ok := PrepareCompaction(manager, opts.keepTurns)
	if !ok {
		return CompactionPreparation{}, "", errors.New("nothing to compact")
	}
	summary, err := summarizeTurns(ctx, gateway, modelID, preparation.DroppedTurns, opts.customInstructions)
	if err != nil {
		return CompactionPreparation{}, "", err
	}
	replacementMessages := buildReplacementMessages(manager.Branch(""), preparation.FirstKeptEntryID, summary, opts.targetTokens)
	if _, err := manager.AppendCompaction(summary, preparation.FirstKeptEntryID, preparation.MessagesBefore, replacementMessages); err != nil {
		return CompactionPreparation{}, "", err
	}
	return preparation, summary, nil
}

func MaybeAutoCompact(ctx context.Context, manager *Manager, cfg config.SessionConfig, gateway model.Client, modelID string) (bool, error) {
	context := manager.BuildContext()
	plan := PlanCompaction(cfg, context.Transcript, model.EstimateMessagesTokens(context.Messages))
	if !plan.ShouldCompact {
		return false, nil
	}
	targetTokens := effectiveCompactionTarget(cfg)
	keepTurns := plan.KeepTurns
	if targetTokens > 0 {
		keepTurns = chooseKeepTurnsForTarget(manager, targetTokens, keepTurns)
	}
	_, _, err := compactWithOptions(ctx, manager, gateway, modelID, compactOptions{
		keepTurns:    keepTurns,
		targetTokens: targetTokens,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func MaybeCompactRequest(
	ctx context.Context,
	manager *Manager,
	cfg config.SessionConfig,
	gateway model.Client,
	req model.Request,
) (model.Request, bool, error) {
	plan := PlanCompaction(cfg, manager.BuildContext().Transcript, model.EstimateRequestTokens(req))
	if !plan.ShouldCompact {
		return req, false, nil
	}
	targetTokens := effectiveCompactionTarget(cfg)
	keepTurns := plan.KeepTurns
	if targetTokens > 0 {
		keepTurns = chooseKeepTurnsForTarget(manager, targetTokens, keepTurns)
	}
	if _, _, err := compactWithOptions(ctx, manager, gateway, req.ModelID, compactOptions{
		keepTurns:          keepTurns,
		targetTokens:       targetTokens,
		customInstructions: "Preserve enough recent context for the next model step.",
	}); err != nil {
		return req, false, err
	}
	context := manager.BuildContext()
	req.Messages = context.Messages
	return req, true, nil
}

func ShouldRetryAfterContextOverflow(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "context") && strings.Contains(text, "overflow")
}

func RetryAfterOverflow(
	ctx context.Context,
	manager *Manager,
	cfg config.SessionConfig,
	gateway model.Client,
	req model.Request,
	retry func(model.Request) (model.Response, error),
) (model.Response, bool, error) {
	if !cfg.Enabled || !cfg.AutoCompact {
		return model.Response{}, false, errors.New("auto-compaction disabled")
	}
	targetTokens := effectiveCompactionTarget(cfg)
	keepTurns := cfg.CompactKeepTurns
	if targetTokens > 0 {
		keepTurns = chooseKeepTurnsForTarget(manager, targetTokens, keepTurns)
	}
	_, _, err := compactWithOptions(ctx, manager, gateway, req.ModelID, compactOptions{
		keepTurns:          keepTurns,
		targetTokens:       targetTokens,
		customInstructions: "The previous model call overflowed context. Preserve enough detail to retry the next model step.",
	})
	if err != nil {
		return model.Response{}, false, err
	}
	context := manager.BuildContext()
	req.Messages = context.Messages
	response, err := retry(req)
	if err != nil {
		return model.Response{}, false, err
	}
	return response, true, nil
}

func GenerateBranchSummary(
	ctx context.Context,
	manager *Manager,
	oldLeafID string,
	targetID string,
	gateway model.Client,
	modelID string,
) (string, error) {
	abandoned := collectAbandonedLines(manager, oldLeafID, targetID)
	if len(abandoned) == 0 {
		return "", errors.New("nothing to summarize")
	}
	var transcript chat.Transcript
	var pendingUser string
	for _, line := range abandoned {
		if line.Type != MessageEntryType {
			continue
		}
		switch line.Kind {
		case UserPromptKind:
			pendingUser = line.Text
		case ModelResponseKind:
			if pendingUser != "" && line.Text != "" {
				transcript = append(transcript, chat.Turn{User: pendingUser, Assistant: line.Text})
				pendingUser = ""
			}
		}
	}
	if len(transcript) == 0 {
		return "", errors.New("nothing to summarize")
	}
	return summarizeTurns(ctx, gateway, modelID, transcript, "Summarize the abandoned branch and preserve anything needed to continue elsewhere.")
}

func summarizeTurns(ctx context.Context, gateway model.Client, modelID string, turns chat.Transcript, customInstructions string) (string, error) {
	promptText := prompt.BuildCompactionPrompt(turns, customInstructions)
	response, err := gateway.Generate(ctx, model.Request{
		SystemPrompt: compactionSystemPrompt,
		ModelID:      modelID,
		Messages:     []model.Message{model.TextMessage(model.UserRole, promptText)},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(response.Text) == "" {
		return "", fmt.Errorf("empty summary")
	}
	return strings.TrimSpace(response.Text), nil
}

func collectAbandonedLines(manager *Manager, oldLeafID string, targetID string) []Line {
	oldBranch := manager.Branch(oldLeafID)
	targetBranch := manager.Branch(targetID)
	common := ""
	for i := 0; i < min(len(oldBranch), len(targetBranch)); i++ {
		if oldBranch[i].ID != targetBranch[i].ID {
			break
		}
		common = oldBranch[i].ID
	}
	var out []Line
	for i := len(oldBranch) - 1; i >= 0; i-- {
		if oldBranch[i].ID == common {
			break
		}
		out = append(out, oldBranch[i])
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func turnStartEntryIDs(branch []Line) []string {
	var ids []string
	for _, line := range branch {
		if line.Type == MessageEntryType && line.Kind == UserPromptKind {
			ids = append(ids, line.ID)
		}
	}
	return ids
}

func buildReplacementMessages(branch []Line, firstKeptEntryID string, summary string, targetTokens int) []model.Message {
	out := []model.Message{model.TextMessage(model.UserRole, prompt.BuildCompactionSummaryMessage(summary))}
	start := 0
	if firstKeptEntryID != "" {
		for i, line := range branch {
			if line.ID == firstKeptEntryID {
				start = i
				break
			}
		}
	}
	for _, line := range branch[start:] {
		switch line.Type {
		case MessageEntryType:
			out = append(out, line.Message)
		case BranchSummaryEntryType:
			out = append(out, model.TextMessage(model.UserRole, "Branch summary:\n"+line.Summary))
		}
	}
	return fitCompactedMessages(out, targetTokens)
}

func chooseKeepTurnsForTarget(manager *Manager, targetTokens int, fallbackKeepTurns int) int {
	if targetTokens <= 0 {
		return fallbackKeepTurns
	}
	context := manager.BuildContext()
	if len(context.Transcript) == 0 {
		return fallbackKeepTurns
	}
	if fallbackKeepTurns <= 0 {
		fallbackKeepTurns = 1
	}
	if fallbackKeepTurns > len(context.Transcript) {
		fallbackKeepTurns = len(context.Transcript)
	}
	chosen := fallbackKeepTurns
	branch := manager.Branch("")
	for keepTurns := fallbackKeepTurns; keepTurns >= 1; keepTurns-- {
		preparation, ok := PrepareCompaction(manager, keepTurns)
		if !ok {
			continue
		}
		replacementMessages := buildReplacementMessages(branch, preparation.FirstKeptEntryID, "summary", targetTokens)
		if model.EstimateMessagesTokens(replacementMessages) <= targetTokens {
			chosen = keepTurns
			break
		}
		chosen = keepTurns
	}
	return chosen
}

func effectiveCompactionTarget(cfg config.SessionConfig) int {
	target := cfg.CompactTargetTokens
	if cfg.CompactThresholdTokens > 0 && (target <= 0 || target > cfg.CompactThresholdTokens) {
		target = cfg.CompactThresholdTokens
	}
	return target
}

func fitCompactedMessages(messages []model.Message, targetTokens int) []model.Message {
	out := append([]model.Message(nil), messages...)
	if targetTokens <= 0 || model.EstimateMessagesTokens(out) <= targetTokens {
		return out
	}

	for i := 1; i < len(out) && model.EstimateMessagesTokens(out) > targetTokens; i++ {
		if isProtectedCompactionIndex(i, len(out)) || !messageHasToolResult(out[i]) {
			continue
		}
		out[i] = model.TextMessage(out[i].Role, "Earlier tool result omitted after compaction to fit token budget.")
	}

	for i := 1; i < len(out) && model.EstimateMessagesTokens(out) > targetTokens; i++ {
		if isProtectedCompactionIndex(i, len(out)) {
			continue
		}
		tokens := model.EstimateMessageTokens(out[i])
		if tokens <= 64 {
			continue
		}
		out[i] = truncateMessageToTokens(out[i], max(16, targetTokens/4))
	}

	for i := 1; i < len(out) && model.EstimateMessagesTokens(out) > targetTokens; i++ {
		if isProtectedCompactionIndex(i, len(out)) {
			continue
		}
		out = append(out[:i], out[i+1:]...)
		i = 0
	}

	for i := 1; i < len(out) && model.EstimateMessagesTokens(out) > targetTokens; i++ {
		if isProtectedCompactionIndex(i, len(out)) {
			out[i] = truncateMessageToTokens(out[i], 24)
		}
	}
	return out
}

func isProtectedCompactionIndex(index int, total int) bool {
	if index <= 0 {
		return true
	}
	return index >= max(1, total-2)
}

func messageHasToolResult(msg model.Message) bool {
	for _, part := range msg.Parts {
		if part.ToolResult != nil {
			return true
		}
	}
	return false
}

func truncateMessageToTokens(msg model.Message, targetTokens int) model.Message {
	if targetTokens <= 0 || model.EstimateMessageTokens(msg) <= targetTokens {
		return msg
	}
	text := strings.TrimSpace(msg.Text())
	if text == "" {
		if messageHasToolResult(msg) {
			return model.TextMessage(msg.Role, "Earlier tool result omitted after compaction to fit token budget.")
		}
		return msg
	}
	limit := max(24, targetTokens*model.ApproxBytesPerToken)
	if len(text) <= limit {
		return msg
	}
	suffix := "\n[truncated after compaction]"
	trimmed := text[:max(0, limit-len(suffix))]
	trimmed = strings.TrimRight(trimmed, " \n\t")
	if trimmed == "" {
		trimmed = text[:min(len(text), limit)]
	}
	return model.TextMessage(msg.Role, trimmed+suffix)
}
