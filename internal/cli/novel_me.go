// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newMeCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "me",
		Short: "Commands about your own activity and status",
	}
	cmd.AddCommand(newMeWeekCmd(flags))
	cmd.AddCommand(newMeExportCmd(flags))
	cmd.AddCommand(newMeSchemaCmd(flags))
	return cmd
}

type weekCheckin struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

type weekObjective struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
}

type weekMeeting struct {
	ID            string `json:"id"`
	Target        string `json:"target"`
	ScheduledDate string `json:"scheduled_date"`
}

type weekSummary struct {
	WeekOf        string          `json:"week_of"`
	SyncedAt      string          `json:"synced_at"`
	Checkins      []weekCheckin   `json:"checkins_this_week"`
	StaleOKRs     []weekObjective `json:"okrs_not_updated_this_week"`
	Upcoming1on1s []weekMeeting   `json:"upcoming_1on1s"`
}

func newMeWeekCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "week",
		Short: "Your week: check-ins that you posted, OKRs to update, upcoming 1:1s",
		Long: `Reads the local SQLite store and shows:
- Check-ins that you posted this week (the week starts on Sunday)
- Open objectives where you are an owner or a stakeholder and that have no check-in this week
- 1:1 meetings that are not completed and start in the next 7 days

The command does not connect to 7Geese. Run 'sync' first to get current data.
The field synced_at shows the age of the local data.

Use --json for structured output.`,
		Example: `  7geese-cli me week
  7geese-cli me week --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database (run '7geese-cli sync' first): %w", err)
			}
			defer db.Close()

			summary, err := buildWeekSummary(db, time.Now())
			if err != nil {
				return err
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(summary)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "My Week (starting %s, data synced %s)\n\n", summary.WeekOf, summary.SyncedAt)

			fmt.Fprintf(w, "Check-ins this week: %d\n", len(summary.Checkins))
			for _, c := range summary.Checkins {
				fmt.Fprintf(w, "  • %s\n", truncate(c.Message, 100))
			}
			if len(summary.Checkins) == 0 {
				fmt.Fprintln(w, "  (none — consider posting a check-in)")
			}

			fmt.Fprintf(w, "\nOKRs not updated this week: %d\n", len(summary.StaleOKRs))
			for _, o := range summary.StaleOKRs {
				fmt.Fprintf(w, "  • %-50s  %.0f%%\n", truncate(o.Name, 50), o.Progress)
			}

			fmt.Fprintf(w, "\nUpcoming 1:1s: %d\n", len(summary.Upcoming1on1s))
			for _, o := range summary.Upcoming1on1s {
				fmt.Fprintf(w, "  • %s on %s\n", o.Target, o.ScheduledDate)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func parseTimestamp(what, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not an ISO 8601 timestamp", what, value)
	}
	return parsed, nil
}

func buildWeekSummary(db *store.Store, now time.Time) (*weekSummary, error) {
	sessionUser, found, err := storedSessionUser(db)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("the local database has no synced data; run '7geese-cli sync'")
	}

	var oldestSync time.Time
	for _, key := range []store.DatasetKey{
		sessionDataset(resourceCheckins),
		sessionDataset(resourceOneonones),
		userDataset(resourceUserObjectives, sessionUser),
	} {
		syncedAt, synced, err := db.DatasetSyncedAt(key)
		if err != nil {
			return nil, err
		}
		if !synced {
			return nil, fmt.Errorf("the dataset %s is not synced; run '7geese-cli sync'", key)
		}
		if oldestSync.IsZero() || syncedAt.Before(oldestSync) {
			oldestSync = syncedAt
		}
	}

	weekStart := time.Date(now.Year(), now.Month(), now.Day()-int(now.Weekday()), 0, 0, 0, 0, now.Location())
	summary := &weekSummary{
		WeekOf:        weekStart.Format(exportDateLayout),
		SyncedAt:      oldestSync.UTC().Format(time.RFC3339),
		Checkins:      []weekCheckin{},
		StaleOKRs:     []weekObjective{},
		Upcoming1on1s: []weekMeeting{},
	}

	checkinRows, err := db.Query(`SELECT id, message, created FROM checkin_view ORDER BY created DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading check-ins: %w", err)
	}
	defer checkinRows.Close()
	for checkinRows.Next() {
		var id sql.NullInt64
		var message, created sql.NullString
		if err := checkinRows.Scan(&id, &message, &created); err != nil {
			return nil, fmt.Errorf("reading a check-in: %w", err)
		}
		if !id.Valid {
			return nil, errors.New("a check-in has no id")
		}
		createdAt, err := parseTimestamp(fmt.Sprintf("the date of check-in %d", id.Int64), created.String)
		if err != nil {
			return nil, err
		}
		if createdAt.Before(weekStart) {
			continue
		}
		summary.Checkins = append(summary.Checkins, weekCheckin{
			ID:        strconv.FormatInt(id.Int64, 10),
			Message:   message.String,
			CreatedAt: created.String,
		})
	}
	if err := checkinRows.Err(); err != nil {
		return nil, fmt.Errorf("reading check-ins: %w", err)
	}
	if err := checkinRows.Close(); err != nil {
		return nil, fmt.Errorf("reading check-ins: %w", err)
	}

	objectiveRows, err := db.Query(`
		SELECT id, name, progress, last_checkin_at
		FROM objective_view
		WHERE owner_id = ? AND closed = 0
		  AND json_extract(data,'$.currentUserParticipantType') IN (1, 2)
		ORDER BY due_datetime IS NULL, due_datetime ASC, id ASC`, sessionUser)
	if err != nil {
		return nil, fmt.Errorf("reading objectives: %w", err)
	}
	defer objectiveRows.Close()
	for objectiveRows.Next() {
		var id sql.NullInt64
		var name, lastCheckin sql.NullString
		var progress sql.NullFloat64
		if err := objectiveRows.Scan(&id, &name, &progress, &lastCheckin); err != nil {
			return nil, fmt.Errorf("reading an objective: %w", err)
		}
		if !id.Valid || !progress.Valid {
			return nil, fmt.Errorf("objective %d has no id or no progress", id.Int64)
		}
		if lastCheckin.Valid {
			lastCheckinAt, err := parseTimestamp(fmt.Sprintf("the last check-in date of objective %d", id.Int64), lastCheckin.String)
			if err != nil {
				return nil, err
			}
			if !lastCheckinAt.Before(weekStart) {
				continue
			}
		}
		summary.StaleOKRs = append(summary.StaleOKRs, weekObjective{
			ID:       strconv.FormatInt(id.Int64, 10),
			Name:     name.String,
			Progress: progress.Float64,
		})
	}
	if err := objectiveRows.Err(); err != nil {
		return nil, fmt.Errorf("reading objectives: %w", err)
	}
	if err := objectiveRows.Close(); err != nil {
		return nil, fmt.Errorf("reading objectives: %w", err)
	}

	horizon := now.AddDate(0, 0, 7)
	meetingRows, err := db.Query(`
		SELECT id, start_datetime, creator_name, target_id, target_name
		FROM oneonone_view
		WHERE status = 0 AND (creator_id = ? OR target_id = ?)
		ORDER BY start_datetime ASC, id ASC`, sessionUser, sessionUser)
	if err != nil {
		return nil, fmt.Errorf("reading 1:1 meetings: %w", err)
	}
	defer meetingRows.Close()
	for meetingRows.Next() {
		var id, targetID sql.NullInt64
		var start, creatorName, targetName sql.NullString
		if err := meetingRows.Scan(&id, &start, &creatorName, &targetID, &targetName); err != nil {
			return nil, fmt.Errorf("reading a 1:1 meeting: %w", err)
		}
		if !id.Valid {
			return nil, errors.New("a 1:1 meeting has no id")
		}
		startAt, err := parseTimestamp(fmt.Sprintf("the start time of 1:1 meeting %d", id.Int64), start.String)
		if err != nil {
			return nil, err
		}
		if startAt.Before(now) || startAt.After(horizon) {
			continue
		}
		with := targetName.String
		if targetID.Valid && targetID.Int64 == int64(sessionUser) {
			with = creatorName.String
		}
		if with == "" {
			return nil, fmt.Errorf("1:1 meeting %d has no name for the other participant", id.Int64)
		}
		summary.Upcoming1on1s = append(summary.Upcoming1on1s, weekMeeting{
			ID:            strconv.FormatInt(id.Int64, 10),
			Target:        with,
			ScheduledDate: start.String,
		})
	}
	if err := meetingRows.Err(); err != nil {
		return nil, fmt.Errorf("reading 1:1 meetings: %w", err)
	}
	return summary, nil
}
