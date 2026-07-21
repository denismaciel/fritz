package trainingplan

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReaderTodayIncludesExactWorkoutSteps(t *testing.T) {
	path := trainingFixture(t)
	reader := NewReader(path)
	now := time.Date(2026, time.July, 21, 8, 30, 0, 0, time.FixedZone("WEST", 3600))

	text, err := reader.Today(context.Background(), now)
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	for _, want := range []string{
		"Training today — Tue 21 Jul",
		"🏃 Run - controlled 4x5",
		"50 min · 8.5 km",
		"Steps:",
		"1. Warm-up · 10 min · HR ≤145 bpm",
		"2. Steady · 5 min · HR 154–162 bpm",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Today() missing %q:\n%s", want, text)
		}
	}
}

func TestReaderWeekUsesMondayThroughSundayAndTotals(t *testing.T) {
	path := trainingFixture(t)
	reader := NewReader(path)
	now := time.Date(2026, time.July, 21, 8, 30, 0, 0, time.UTC)

	text, err := reader.Week(context.Background(), now)
	if err != nil {
		t.Fatalf("Week() error = %v", err)
	}
	for _, want := range []string{
		"Training week — Mon 20 Jul–Sun 26 Jul",
		"Mon 20 Jul",
		"✅ 🚴 Ride - aerobic",
		"Tue 21 Jul",
		"Run - controlled 4x5",
		"Total: 2 workouts · 2h10 · 38.5 km",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Week() missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Mon 27 Jul") {
		t.Fatalf("Week() included next week:\n%s", text)
	}
}

func TestReaderTodayWithoutPlanIsExplicit(t *testing.T) {
	path := trainingFixture(t)
	reader := NewReader(path)

	text, err := reader.Today(context.Background(), time.Date(2026, time.July, 24, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Today() error = %v", err)
	}
	if !strings.Contains(text, "No training is stored for today") {
		t.Fatalf("Today() = %q", text)
	}
}

func trainingFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "training.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	schema := []string{
		`CREATE TABLE planned_sessions (
			id INTEGER PRIMARY KEY,
			date DATE NOT NULL,
			title TEXT NOT NULL,
			notes TEXT,
			completed BOOLEAN NOT NULL
		)`,
		`CREATE TABLE planned_session_workouts (
			id INTEGER PRIMARY KEY,
			session_id INTEGER NOT NULL,
			sequence INTEGER NOT NULL,
			sport TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT NOT NULL,
			duration_seconds INTEGER,
			distance_meters REAL,
			completed BOOLEAN NOT NULL
		)`,
		`CREATE TABLE planned_workout_steps (
			id INTEGER PRIMARY KEY,
			workout_id INTEGER NOT NULL,
			sequence INTEGER NOT NULL,
			kind TEXT NOT NULL,
			duration_seconds INTEGER,
			distance_meters REAL,
			target_type TEXT,
			target_min REAL,
			target_max REAL,
			target_unit TEXT,
			cue TEXT
		)`,
		`INSERT INTO planned_sessions VALUES
			(1, '2026-07-20', 'Ride', '', 1),
			(2, '2026-07-21', 'Run', '', 0),
			(3, '2026-07-27', 'Next week', '', 0)`,
		`INSERT INTO planned_session_workouts VALUES
			(10, 1, 1, 'Ride', 'Ride - aerobic', 'Long aerobic ride.', 4800, 30000, 1),
			(20, 2, 1, 'Run', 'Run - controlled 4x5', 'Controlled quality session.', 3000, 8500, 0),
			(30, 3, 1, 'Run', 'Run - next week', 'Do not include.', 2400, 6000, 0)`,
		`INSERT INTO planned_workout_steps VALUES
			(1, 20, 1, 'warmup', 600, NULL, 'hr_bpm', NULL, 145, 'bpm', 'Easy'),
			(2, 20, 2, 'steady', 300, NULL, 'hr_bpm', 154, 162, 'bpm', 'Controlled strong')`,
	}
	for _, statement := range schema {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("fixture exec error = %v", err)
		}
	}
	return path
}
