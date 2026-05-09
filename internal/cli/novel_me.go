// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
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

func newMeWeekCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "week",
		Short: "Everything relevant to you this week: check-ins due, OKRs to update, upcoming 1:1s",
		Long: `Queries the local SQLite store and surfaces:
- Check-ins you posted this week
- Open objectives not updated this week
- Upcoming 1:1s in the next 7 days

Use --json for agent-friendly structured output.`,
		Example: `  7geese-cli me week
  7geese-cli me week --json`,
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

			now := time.Now()
			weekStart := now.AddDate(0, 0, -int(now.Weekday())).Format("2006-01-02")
			weekEnd := now.AddDate(0, 0, 7-int(now.Weekday())).Format("2006-01-02")

			// Check-ins this week
			checkinRows, err := db.Query(`
				SELECT json_extract(data,'$.id'), json_extract(data,'$.message'), json_extract(data,'$.created_at')
				FROM resources
				WHERE resource_type = 'checkins'
				  AND json_extract(data,'$.created_at') >= ?
				ORDER BY json_extract(data,'$.created_at') DESC
			`, weekStart)
			if err != nil {
				return fmt.Errorf("querying check-ins: %w", err)
			}
			type checkinItem struct {
				ID        string `json:"id"`
				Message   string `json:"message"`
				CreatedAt string `json:"created_at"`
			}
			var checkins []checkinItem
			for checkinRows.Next() {
				var c checkinItem
				if err := checkinRows.Scan(&c.ID, &c.Message, &c.CreatedAt); err != nil {
					continue
				}
				if len(c.Message) > 100 {
					c.Message = c.Message[:97] + "..."
				}
				checkins = append(checkins, c)
			}
			checkinRows.Close()

			// OKRs not updated this week
			staleOKRRows, err := db.Query(`
				SELECT json_extract(data,'$.id'), json_extract(data,'$.name'), json_extract(data,'$.progress')
				FROM resources
				WHERE resource_type IN ('objectives','teamobjectives')
				  AND COALESCE(json_extract(data,'$.closed'),0) = 0
				  AND (json_extract(data,'$.updated_at') < ? OR json_extract(data,'$.updated_at') IS NULL)
				ORDER BY json_extract(data,'$.due_date') ASC
				LIMIT 10
			`, weekStart)
			if err != nil {
				return fmt.Errorf("querying OKRs: %w", err)
			}
			type okrItem struct {
				ID       string  `json:"id"`
				Name     string  `json:"name"`
				Progress float64 `json:"progress"`
			}
			var staleOKRs []okrItem
			for staleOKRRows.Next() {
				var o okrItem
				if err := staleOKRRows.Scan(&o.ID, &o.Name, &o.Progress); err != nil {
					continue
				}
				staleOKRs = append(staleOKRs, o)
			}
			staleOKRRows.Close()

			// Upcoming 1:1s this week
			oneononeRows, err := db.Query(`
				SELECT json_extract(data,'$.id'), json_extract(data,'$.target'), json_extract(data,'$.scheduled_date')
				FROM resources
				WHERE resource_type = 'oneonones'
				  AND json_extract(data,'$.status') = 'upcoming'
				  AND json_extract(data,'$.scheduled_date') BETWEEN ? AND ?
				ORDER BY json_extract(data,'$.scheduled_date') ASC
			`, weekStart, weekEnd)
			if err != nil {
				return fmt.Errorf("querying 1:1s: %w", err)
			}
			type oneononeItem struct {
				ID            string `json:"id"`
				Target        string `json:"target"`
				ScheduledDate string `json:"scheduled_date"`
			}
			var oneonones []oneononeItem
			for oneononeRows.Next() {
				var o oneononeItem
				if err := oneononeRows.Scan(&o.ID, &o.Target, &o.ScheduledDate); err != nil {
					continue
				}
				oneonones = append(oneonones, o)
			}
			oneononeRows.Close()

			type weekSummary struct {
				WeekOf        string         `json:"week_of"`
				Checkins      []checkinItem  `json:"checkins_this_week"`
				StaleOKRs     []okrItem      `json:"okrs_not_updated_this_week"`
				Upcoming1on1s []oneononeItem `json:"upcoming_1on1s"`
			}
			summary := weekSummary{
				WeekOf:        weekStart,
				Checkins:      checkins,
				StaleOKRs:     staleOKRs,
				Upcoming1on1s: oneonones,
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(summary)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "My Week (starting %s)\n\n", weekStart)

			fmt.Fprintf(w, "Check-ins this week: %d\n", len(checkins))
			for _, c := range checkins {
				fmt.Fprintf(w, "  • %s\n", c.Message)
			}
			if len(checkins) == 0 {
				fmt.Fprintln(w, "  (none — consider posting a check-in)")
			}

			fmt.Fprintf(w, "\nOKRs not updated this week: %d\n", len(staleOKRs))
			for _, o := range staleOKRs {
				fmt.Fprintf(w, "  • %-50s  %.0f%%\n", truncate(o.Name, 50), o.Progress)
			}

			fmt.Fprintf(w, "\nUpcoming 1:1s: %d\n", len(oneonones))
			for _, o := range oneonones {
				fmt.Fprintf(w, "  • %s on %s\n", o.Target, o.ScheduledDate)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
