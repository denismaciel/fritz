package session

import (
	"fritz/internal/chat"
	"fritz/internal/config"
)

type Session struct {
	Transcript chat.Transcript `json:"transcript"`
}

type StartMode string

const (
	StartNew       StartMode = "new"
	ResumeExisting StartMode = "resume"
)

func DecideStartMode(hasSavedSession bool, resumeRequested bool) StartMode {
	if hasSavedSession && resumeRequested {
		return ResumeExisting
	}
	return StartNew
}

type PersistencePlan struct {
	Enabled       bool
	ShouldPersist bool
}

func PlanPersistence(cfg config.SessionConfig, transcript chat.Transcript) PersistencePlan {
	return PersistencePlan{
		Enabled:       cfg.Enabled,
		ShouldPersist: cfg.Enabled && len(transcript) > 0,
	}
}

type CompactionPlan struct {
	ShouldCompact bool
	DropTurns     int
	KeepTurns     int
	Reason        string
}

func PlanCompaction(cfg config.SessionConfig, transcript chat.Transcript, estimatedTokens int) CompactionPlan {
	if !cfg.Enabled || !cfg.AutoCompact {
		return CompactionPlan{KeepTurns: cfg.CompactKeepTurns}
	}
	if cfg.CompactThresholdTokens > 0 && estimatedTokens >= cfg.CompactThresholdTokens {
		drop := len(transcript) - cfg.CompactKeepTurns
		if drop < 0 {
			drop = 0
		}
		return CompactionPlan{
			ShouldCompact: drop > 0,
			DropTurns:     drop,
			KeepTurns:     cfg.CompactKeepTurns,
			Reason:        "token_threshold",
		}
	}
	if len(transcript) < cfg.CompactThresholdTurns {
		return CompactionPlan{KeepTurns: cfg.CompactKeepTurns}
	}

	drop := len(transcript) - cfg.CompactKeepTurns
	if drop < 0 {
		drop = 0
	}

	return CompactionPlan{
		ShouldCompact: drop > 0,
		DropTurns:     drop,
		KeepTurns:     cfg.CompactKeepTurns,
		Reason:        "turn_threshold",
	}
}
