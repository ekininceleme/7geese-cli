// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newObjectivesStaleCmd(flags *rootFlags) *cobra.Command {
	var days int
	var dbPath string

	cmd := &cobra.Command{
		Use:   "stale",
		Short: "Find objectives not updated in N days",
		Long: `Queries the local SQLite store for objectives whose updated_at timestamp
is older than --days. Useful to find OKRs at risk of abandonment before a
performance cycle closes.

Run 'sync' first to populate the local store.`,
		Example: `  7geese-cli objectives stale --days 14
  7geese-cli objectives stale --days 7 --json`,
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

			cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02T15:04:05")

			rows, err := db.Query(`
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.progress'),
				       json_extract(data,'$.updated_at'), json_extract(data,'$.due_date'),
				       resource_type
				FROM resources
				WHERE resource_type IN ('objectives','teamobjectives','organizationalobjectives')
				  AND COALESCE(json_extract(data,'$.closed'),0) = 0
				  AND (json_extract(data,'$.updated_at') < ? OR json_extract(data,'$.updated_at') IS NULL)
				ORDER BY json_extract(data,'$.updated_at') ASC
			`, cutoff)
			if err != nil {
				return fmt.Errorf("querying stale objectives: %w", err)
			}
			defer rows.Close()

			type staleRow struct {
				ID              string  `json:"id"`
				Name            string  `json:"name"`
				Progress        float64 `json:"progress"`
				UpdatedAt       string  `json:"updated_at"`
				DueDate         string  `json:"due_date"`
				Level           string  `json:"level"`
				DaysSinceUpdate int     `json:"days_since_update"`
			}

			var results []staleRow
			now := time.Now()
			for rows.Next() {
				var id, name, updatedAt, dueDate, resType string
				var progress float64
				if err := rows.Scan(&id, &name, &progress, &updatedAt, &dueDate, &resType); err != nil {
					continue
				}
				daysSince := days
				if updatedAt != "" {
					for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
						if t, err := time.Parse(layout, updatedAt); err == nil {
							daysSince = int(now.Sub(t).Hours() / 24)
							break
						}
					}
				}
				results = append(results, staleRow{
					ID:              id,
					Name:            name,
					Progress:        progress,
					UpdatedAt:       updatedAt,
					DueDate:         dueDate,
					Level:           resType,
					DaysSinceUpdate: daysSince,
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating results: %w", err)
			}

			if len(results) == 0 {
				fmt.Fprintf(os.Stderr, "No objectives stale for more than %d days.\n", days)
				return nil
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d objectives not updated in %d+ days:\n\n", len(results), days)
			for _, r := range results {
				fmt.Fprintf(w, "  %s  (%.0f%%, %d days ago)\n", truncate(r.Name, 60), r.Progress, r.DaysSinceUpdate)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&days, "days", 14, "Consider objectives stale if not updated in this many days")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
