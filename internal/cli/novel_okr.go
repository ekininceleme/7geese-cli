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

func newOKRCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "okr",
		Short: "OKR health and analysis commands",
	}
	cmd.AddCommand(newOKRHealthCmd(flags))
	return cmd
}

func newOKRHealthCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Show OKR health: on track, at risk, or stale across personal, team, and org levels",
		Long: `Queries the local SQLite store (populated by 'sync') and classifies each
objective by its progress and due date proximity.

Health statuses:
  on_track  — progress >= 70% or due date is more than 14 days out
  at_risk   — progress 30-69% and due within 14 days
  stale     — not updated in 14+ days regardless of progress
  overdue   — due date has passed and progress < 100%`,
		Example: `  7geese-cli sync
  7geese-cli okr health
  7geese-cli okr health --json`,
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

			rows, err := db.Query(`
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.progress'),
				       json_extract(data,'$.due_date'), json_extract(data,'$.updated_at'),
				       json_extract(data,'$.objective_type'), resource_type
				FROM resources
				WHERE resource_type IN ('objectives','teamobjectives','organizationalobjectives')
				  AND COALESCE(json_extract(data,'$.closed'),0) = 0
				ORDER BY resource_type, json_extract(data,'$.name')
			`)
			if err != nil {
				return fmt.Errorf("querying objectives: %w", err)
			}
			defer rows.Close()

			type healthRow struct {
				ID             string  `json:"id"`
				Name           string  `json:"name"`
				Progress       float64 `json:"progress"`
				DueDate        string  `json:"due_date"`
				UpdatedAt      string  `json:"updated_at"`
				ObjectiveType  string  `json:"objective_type"`
				Level          string  `json:"level"`
				Health         string  `json:"health"`
				DaysUntilDue   *int    `json:"days_until_due,omitempty"`
				DaysSinceUpdate int    `json:"days_since_update"`
			}

			var results []healthRow
			now := time.Now()

			for rows.Next() {
				var id, name, dueDate, updatedAt, objType, resType string
				var progress float64
				if err := rows.Scan(&id, &name, &progress, &dueDate, &updatedAt, &objType, &resType); err != nil {
					continue
				}

				level := resType
				if objType != "" {
					level = objType
				}

				// Compute days since last update
				daysSinceUpdate := 0
				if updatedAt != "" {
					for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
						if t, err := time.Parse(layout, updatedAt); err == nil {
							daysSinceUpdate = int(now.Sub(t).Hours() / 24)
							break
						}
					}
				}

				// Compute days until due
				var daysUntilDue *int
				if dueDate != "" {
					for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05Z"} {
						if t, err := time.Parse(layout, dueDate); err == nil {
							d := int(t.Sub(now).Hours() / 24)
							daysUntilDue = &d
							break
						}
					}
				}

				health := classifyOKRHealth(progress, daysUntilDue, daysSinceUpdate)

				results = append(results, healthRow{
					ID:              id,
					Name:            name,
					Progress:        progress,
					DueDate:         dueDate,
					UpdatedAt:       updatedAt,
					ObjectiveType:   objType,
					Level:           level,
					Health:          health,
					DaysUntilDue:    daysUntilDue,
					DaysSinceUpdate: daysSinceUpdate,
				})
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating objectives: %w", err)
			}

			if len(results) == 0 {
				fmt.Fprintln(os.Stderr, "No open objectives found. Run 'sync' first.")
				return nil
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			// Human-readable table
			w := cmd.OutOrStdout()
			counts := map[string]int{}
			for _, r := range results {
				counts[r.Health]++
			}
			fmt.Fprintf(w, "OKR Health Summary: %d on_track  %d at_risk  %d stale  %d overdue\n\n",
				counts["on_track"], counts["at_risk"], counts["stale"], counts["overdue"])

			for _, r := range results {
				icon := healthIcon(r.Health)
				due := ""
				if r.DaysUntilDue != nil {
					if *r.DaysUntilDue < 0 {
						due = fmt.Sprintf(" [%d days overdue]", -*r.DaysUntilDue)
					} else {
						due = fmt.Sprintf(" [due in %d days]", *r.DaysUntilDue)
					}
				}
				fmt.Fprintf(w, "%s %-50s  %.0f%%%s\n", icon, truncate(r.Name, 50), r.Progress, due)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func classifyOKRHealth(progress float64, daysUntilDue *int, daysSinceUpdate int) string {
	if daysUntilDue != nil && *daysUntilDue < 0 && progress < 100 {
		return "overdue"
	}
	if daysSinceUpdate >= 14 {
		return "stale"
	}
	if daysUntilDue != nil && *daysUntilDue <= 14 && progress < 70 {
		return "at_risk"
	}
	return "on_track"
}

func healthIcon(health string) string {
	switch health {
	case "on_track":
		return green("✓")
	case "at_risk":
		return yellow("!")
	case "stale":
		return yellow("~")
	case "overdue":
		return red("✗")
	default:
		return " "
	}
}


