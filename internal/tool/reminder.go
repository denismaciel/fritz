package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fritz/internal/reminder"
)

type reminderSetTool struct {
	store *reminder.Store
	now   func() time.Time
}

type reminderListTool struct {
	store *reminder.Store
}

type reminderDeleteTool struct {
	store *reminder.Store
}

func NewReminderSetTool(cwd string) Tool {
	return NewReminderSetToolWithNow(cwd, func() time.Time { return time.Now().UTC() })
}

func NewReminderSetToolWithNow(cwd string, now func() time.Time) Tool {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return reminderSetTool{store: reminder.NewStore(cwd), now: now}
}

func NewReminderListTool(cwd string) Tool {
	return reminderListTool{store: reminder.NewStore(cwd)}
}

func NewReminderDeleteTool(cwd string) Tool {
	return reminderDeleteTool{store: reminder.NewStore(cwd)}
}

func (t reminderSetTool) Definition() Definition {
	return Definition{
		Name:          "reminder_set",
		Description:   "Create a reminder tied to the current conversation session.",
		PromptSnippet: "Create durable reminders that heartbeat can deliver later",
		PromptGuidelines: []string{
			"Use reminder_set when the user asks to be reminded later.",
			"Prefer relative durations like 30m, 2h, or RFC3339 timestamps in UTC for reminder time.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"when": {Type: "string", Description: "Reminder time as RFC3339 UTC timestamp or relative duration like 2h"},
				"text": {Type: "string", Description: "What to remind the user about"},
			},
			Required: []string{"when", "text"},
		},
	}
}

func (t reminderSetTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	whenRaw, errResult, err := requireStringArg("", call, "when")
	if err != nil {
		return errResult, err
	}
	text, errResult, err := requireStringArg("", call, "text")
	if err != nil {
		return errResult, err
	}
	run := CurrentRunContext(ctx)
	if strings.TrimSpace(run.SessionPath) == "" {
		err := fmt.Errorf("reminders require a persisted session")
		return errorResult(call, err), err
	}
	when, err := parseReminderWhen(t.now(), whenRaw)
	if err != nil {
		return errorResult(call, err), err
	}
	item, err := t.store.Set(t.now(), reminder.ReminderInput{
		SessionPath: run.SessionPath,
		When:        when,
		Text:        text,
	})
	if err != nil {
		return errorResult(call, err), err
	}
	return TextResult(call, fmt.Sprintf("Reminder %q set for %s.", item.ID, item.When)), nil
}

func (t reminderListTool) Definition() Definition {
	return Definition{
		Name:          "reminder_list",
		Description:   "List stored reminders.",
		PromptSnippet: "List stored reminders and their due times",
		PromptGuidelines: []string{
			"Use reminder_list to inspect active reminders before adding duplicates.",
		},
		Parameters: Parameters{
			Type:       "object",
			Properties: map[string]Property{},
		},
	}
}

func (t reminderListTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	items, err := t.store.List()
	if err != nil {
		return errorResult(call, err), err
	}
	if len(items) == 0 {
		return TextResult(call, "No reminders."), nil
	}
	var builder strings.Builder
	builder.WriteString("Reminders:\n")
	for _, item := range items {
		builder.WriteString("- ")
		builder.WriteString(item.ID)
		builder.WriteString(" at ")
		builder.WriteString(item.When)
		builder.WriteString(": ")
		builder.WriteString(item.Text)
		if strings.TrimSpace(item.TriggeredAt) != "" {
			builder.WriteString(" [triggered]")
		}
		if strings.TrimSpace(item.SessionPath) != "" {
			builder.WriteString(" (")
			builder.WriteString(item.SessionPath)
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	return TextResult(call, strings.TrimSpace(builder.String())), nil
}

func (t reminderDeleteTool) Definition() Definition {
	return Definition{
		Name:          "reminder_delete",
		Description:   "Delete a reminder by id.",
		PromptSnippet: "Delete reminders by id",
		PromptGuidelines: []string{
			"Use reminder_delete when asked to cancel a reminder.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"id": {Type: "string", Description: "Reminder id"},
			},
			Required: []string{"id"},
		},
	}
}

func (t reminderDeleteTool) Run(ctx context.Context, call Call) (Result, error) {
	if err := ctx.Err(); err != nil {
		return errorResult(call, err), err
	}
	id, errResult, err := requireStringArg("", call, "id")
	if err != nil {
		return errResult, err
	}
	deleted, err := t.store.Delete(id)
	if err != nil {
		return errorResult(call, err), err
	}
	if !deleted {
		return TextResult(call, fmt.Sprintf("Reminder %q not found.", id)), nil
	}
	return TextResult(call, fmt.Sprintf("Deleted reminder %q.", id)), nil
}

func parseReminderWhen(now time.Time, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("missing reminder time")
	}
	if when, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return when.UTC(), nil
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		return now.UTC().Add(duration), nil
	}
	return time.Time{}, fmt.Errorf("invalid reminder time %q", raw)
}
