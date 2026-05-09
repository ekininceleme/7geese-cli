// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newCheckinsStreakCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var userID string

	cmd := &cobra.Command{
		Use:   "streak",
		Short: "Show consecutive weekly check-in streaks",
		Long: `Queries the local SQLite store (populated by 'sync') to compute consecutive
weekly check-in streaks. A "week" is ISO week (Monday–Sunday).

A streak is broken when there is no check-in in a calendar week.`,
		Example: `  7geese-cli checkins streak
  7geese-cli checkins streak --user 42 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database (run 'sync' first): %w", err)
			}
			defer db.Close()

			query := `
				SELECT json_extract(data,'$.created_at'), json_extract(data,'$.creator')
				FROM resources
				WHERE resource_type = 'checkins'
				ORDER BY json_extract(data,'$.created_at') DESC
			`
			var queryArgs []any
			if userID != "" {
				query = `
					SELECT json_extract(data,'$.created_at'), json_extract(data,'$.creator')
					FROM resources
					WHERE resource_type = 'checkins'
					  AND (json_extract(data,'$.creator') = ? OR json_extract(data,'$.creator') LIKE ?)
					ORDER BY json_extract(data,'$.created_at') DESC
				`
				queryArgs = []any{userID, "%/" + userID + "/"}
			}

			rows, err := db.Query(query, queryArgs...)
			if err != nil {
				return fmt.Errorf("querying check-ins: %w", err)
			}
			defer rows.Close()

			// Collect distinct ISO weeks with check-ins
			weeksSeen := map[string]bool{}
			for rows.Next() {
				var createdAt, creator string
				if err := rows.Scan(&createdAt, &creator); err != nil {
					continue
				}
				for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
					if t, err := time.Parse(layout, createdAt); err == nil {
						year, week := t.ISOWeek()
						weeksSeen[fmt.Sprintf("%d-W%02d", year, week)] = true
						break
					}
				}
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating check-ins: %w", err)
			}

			streak := computeWeekStreak(weeksSeen)

			type streakResult struct {
				CurrentStreak  int      `json:"current_streak_weeks"`
				TotalWeeks     int      `json:"total_weeks_with_checkins"`
				ActiveWeeks    []string `json:"active_weeks"`
			}
			activeWeeks := make([]string, 0, len(weeksSeen))
			for w := range weeksSeen {
				activeWeeks = append(activeWeeks, w)
			}

			result := streakResult{
				CurrentStreak: streak,
				TotalWeeks:    len(weeksSeen),
				ActiveWeeks:   activeWeeks,
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Current streak: %d consecutive week(s) with check-ins\n", streak)
			fmt.Fprintf(w, "Total weeks with check-ins: %d\n", len(weeksSeen))
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&userID, "user", "", "Filter by user ID or URI")
	return cmd
}

// computeWeekStreak counts consecutive ISO weeks ending at the current week.
func computeWeekStreak(weeksSeen map[string]bool) int {
	now := time.Now()
	streak := 0
	for {
		year, week := now.ISOWeek()
		key := fmt.Sprintf("%d-W%02d", year, week)
		if !weeksSeen[key] {
			break
		}
		streak++
		now = now.AddDate(0, 0, -7)
	}
	return streak
}
