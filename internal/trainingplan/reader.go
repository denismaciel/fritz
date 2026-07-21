package trainingplan

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Reader struct {
	path string
}

type workout struct {
	id              int64
	date            time.Time
	sport           string
	title           string
	description     string
	durationSeconds sql.NullInt64
	distanceMeters  sql.NullFloat64
	completed       bool
	steps           []step
}

type step struct {
	sequence        int
	kind            string
	durationSeconds sql.NullInt64
	distanceMeters  sql.NullFloat64
	targetType      sql.NullString
	targetMin       sql.NullFloat64
	targetMax       sql.NullFloat64
	targetUnit      sql.NullString
	cue             sql.NullString
}

func NewReader(path string) *Reader {
	return &Reader{path: strings.TrimSpace(path)}
}

func (r *Reader) Today(ctx context.Context, now time.Time) (string, error) {
	day := dateOnly(now)
	workouts, err := r.workouts(ctx, day, day, true)
	if err != nil {
		return "", err
	}
	return renderToday(day, workouts), nil
}

func (r *Reader) Week(ctx context.Context, now time.Time) (string, error) {
	day := dateOnly(now)
	start := startOfWeek(day)
	end := start.AddDate(0, 0, 6)
	workouts, err := r.workouts(ctx, start, end, false)
	if err != nil {
		return "", err
	}
	return renderWeek(start, end, workouts), nil
}

func (r *Reader) workouts(ctx context.Context, start, end time.Time, includeSteps bool) ([]workout, error) {
	if strings.TrimSpace(r.path) == "" {
		return nil, fmt.Errorf("training database is not configured")
	}
	db, err := sql.Open("sqlite3", readOnlyDSN(r.path))
	if err != nil {
		return nil, fmt.Errorf("open training database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	rows, err := db.QueryContext(ctx, `
		SELECT
			w.id,
			CAST(p.date AS TEXT),
			w.sport,
			w.title,
			w.description,
			w.duration_seconds,
			w.distance_meters,
			w.completed
		FROM planned_session_workouts AS w
		JOIN planned_sessions AS p ON p.id = w.session_id
		WHERE p.date >= ? AND p.date <= ?
		ORDER BY p.date ASC, w.sequence ASC, w.id ASC
	`, start.Format(time.DateOnly), end.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("query training plan: %w", err)
	}
	defer rows.Close()

	var workouts []workout
	for rows.Next() {
		var item workout
		var dateText string
		if err := rows.Scan(
			&item.id,
			&dateText,
			&item.sport,
			&item.title,
			&item.description,
			&item.durationSeconds,
			&item.distanceMeters,
			&item.completed,
		); err != nil {
			return nil, fmt.Errorf("scan training plan: %w", err)
		}
		item.date, err = time.ParseInLocation(time.DateOnly, dateText, start.Location())
		if err != nil {
			return nil, fmt.Errorf("parse training date %q: %w", dateText, err)
		}
		workouts = append(workouts, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read training plan: %w", err)
	}
	if !includeSteps || len(workouts) == 0 {
		return workouts, nil
	}

	stepQuery, err := db.PrepareContext(ctx, `
		SELECT
			sequence,
			kind,
			duration_seconds,
			distance_meters,
			target_type,
			target_min,
			target_max,
			target_unit,
			cue
		FROM planned_workout_steps
		WHERE workout_id = ?
		ORDER BY sequence ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("prepare training steps: %w", err)
	}
	defer stepQuery.Close()

	for i := range workouts {
		stepRows, err := stepQuery.QueryContext(ctx, workouts[i].id)
		if err != nil {
			return nil, fmt.Errorf("query training steps: %w", err)
		}
		for stepRows.Next() {
			var item step
			if err := stepRows.Scan(
				&item.sequence,
				&item.kind,
				&item.durationSeconds,
				&item.distanceMeters,
				&item.targetType,
				&item.targetMin,
				&item.targetMax,
				&item.targetUnit,
				&item.cue,
			); err != nil {
				stepRows.Close()
				return nil, fmt.Errorf("scan training steps: %w", err)
			}
			workouts[i].steps = append(workouts[i].steps, item)
		}
		if err := stepRows.Err(); err != nil {
			stepRows.Close()
			return nil, fmt.Errorf("read training steps: %w", err)
		}
		stepRows.Close()
	}
	return workouts, nil
}

func readOnlyDSN(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	value := url.URL{Scheme: "file", Path: absolute}
	query := value.Query()
	query.Set("mode", "ro")
	query.Set("_busy_timeout", "5000")
	value.RawQuery = query.Encode()
	return value.String()
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func startOfWeek(day time.Time) time.Time {
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}

func renderToday(day time.Time, workouts []workout) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Training today — %s", day.Format("Mon 2 Jan"))
	if len(workouts) == 0 {
		fmt.Fprintf(&out, "\n\nNo training is stored for today. Try /training week.")
		return out.String()
	}
	for _, item := range workouts {
		out.WriteString("\n\n")
		out.WriteString(workoutHeading(item))
		if summary := workoutSummary(item); summary != "" {
			out.WriteString("\n")
			out.WriteString(summary)
		}
		if description := strings.TrimSpace(item.description); description != "" {
			out.WriteString("\n")
			out.WriteString(description)
		}
		if len(item.steps) > 0 {
			out.WriteString("\n\nSteps:")
			for _, workoutStep := range item.steps {
				fmt.Fprintf(&out, "\n%d. %s", workoutStep.sequence, renderStep(workoutStep))
			}
		}
	}
	return out.String()
}

func renderWeek(start, end time.Time, workouts []workout) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Training week — %s–%s", start.Format("Mon 2 Jan"), end.Format("Mon 2 Jan"))
	if len(workouts) == 0 {
		out.WriteString("\n\nNo training is stored for this week.")
		return out.String()
	}

	var currentDay string
	var totalSeconds int64
	var totalMeters float64
	for _, item := range workouts {
		day := item.date.Format("Mon 2 Jan")
		if day != currentDay {
			fmt.Fprintf(&out, "\n\n%s", day)
			currentDay = day
		}
		fmt.Fprintf(&out, "\n%s", workoutHeading(item))
		if summary := workoutSummary(item); summary != "" {
			fmt.Fprintf(&out, " · %s", summary)
		}
		if description := strings.TrimSpace(item.description); description != "" {
			fmt.Fprintf(&out, "\n%s", description)
		}
		if item.durationSeconds.Valid {
			totalSeconds += item.durationSeconds.Int64
		}
		if item.distanceMeters.Valid {
			totalMeters += item.distanceMeters.Float64
		}
	}

	fmt.Fprintf(&out, "\n\nTotal: %d workouts", len(workouts))
	if totalSeconds > 0 {
		fmt.Fprintf(&out, " · %s", formatDuration(totalSeconds))
	}
	if totalMeters > 0 {
		fmt.Fprintf(&out, " · %s", formatDistance(totalMeters))
	}
	return out.String()
}

func workoutHeading(item workout) string {
	status := ""
	if item.completed {
		status = "✅ "
	}
	icon := sportIcon(item.sport)
	title := strings.TrimSpace(item.title)
	if title == "" {
		title = strings.TrimSpace(item.sport)
	}
	if icon != "" {
		return status + icon + " " + title
	}
	return status + title
}

func workoutSummary(item workout) string {
	var parts []string
	if item.durationSeconds.Valid && item.durationSeconds.Int64 > 0 {
		parts = append(parts, formatDuration(item.durationSeconds.Int64))
	}
	if item.distanceMeters.Valid && item.distanceMeters.Float64 > 0 {
		parts = append(parts, formatDistance(item.distanceMeters.Float64))
	}
	return strings.Join(parts, " · ")
}

func renderStep(item step) string {
	kind := strings.ReplaceAll(strings.TrimSpace(item.kind), "_", " ")
	var label string
	switch strings.ToLower(kind) {
	case "warmup", "warm up":
		label = "Warm-up"
	case "cooldown", "cool down":
		label = "Cool-down"
	case "":
		label = "Step"
	default:
		label = strings.ToUpper(kind[:1]) + kind[1:]
	}
	var details []string
	if item.durationSeconds.Valid && item.durationSeconds.Int64 > 0 {
		details = append(details, formatDuration(item.durationSeconds.Int64))
	}
	if item.distanceMeters.Valid && item.distanceMeters.Float64 > 0 {
		details = append(details, formatDistance(item.distanceMeters.Float64))
	}
	if target := renderTarget(item); target != "" {
		details = append(details, target)
	}
	if len(details) > 0 {
		label += " · " + strings.Join(details, " · ")
	}
	if cue := strings.TrimSpace(item.cue.String); item.cue.Valid && cue != "" {
		label += " — " + cue
	}
	return label
}

func renderTarget(item step) string {
	if !item.targetType.Valid {
		return ""
	}
	unit := strings.TrimSpace(item.targetUnit.String)
	switch strings.TrimSpace(item.targetType.String) {
	case "hr_bpm":
		return formatRange("HR ", item.targetMin, item.targetMax, " bpm")
	case "power_watts":
		return formatRange("", item.targetMin, item.targetMax, " W")
	case "hr_zone":
		return formatRange("Z", item.targetMin, item.targetMax, "")
	default:
		if unit != "" {
			unit = " " + unit
		}
		return formatRange("", item.targetMin, item.targetMax, unit)
	}
}

func formatRange(prefix string, min, max sql.NullFloat64, suffix string) string {
	switch {
	case min.Valid && max.Valid:
		if min.Float64 == max.Float64 {
			return prefix + formatNumber(min.Float64) + suffix
		}
		return prefix + formatNumber(min.Float64) + "–" + formatNumber(max.Float64) + suffix
	case max.Valid:
		return prefix + "≤" + formatNumber(max.Float64) + suffix
	case min.Valid:
		return prefix + "≥" + formatNumber(min.Float64) + suffix
	default:
		return ""
	}
}

func formatDuration(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	if seconds%3600 == 0 {
		return fmt.Sprintf("%dh", seconds/3600)
	}
	if seconds >= 3600 {
		return fmt.Sprintf("%dh%02d", seconds/3600, (seconds%3600)/60)
	}
	if seconds%60 == 0 {
		return fmt.Sprintf("%d min", seconds/60)
	}
	return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
}

func formatDistance(meters float64) string {
	if meters >= 1000 {
		return formatNumber(meters/1000) + " km"
	}
	return formatNumber(meters) + " m"
}

func formatNumber(value float64) string {
	if value == float64(int64(value)) {
		return fmt.Sprintf("%d", int64(value))
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
}

func sportIcon(sport string) string {
	switch strings.ToLower(strings.TrimSpace(sport)) {
	case "run", "running":
		return "🏃"
	case "ride", "cycling", "bike":
		return "🚴"
	case "swim", "swimming":
		return "🏊"
	case "strength", "weighttraining", "weight_training":
		return "🏋️"
	default:
		return ""
	}
}
