// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newManagerCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "Manager-focused commands",
	}
	cmd.AddCommand(newManagerDashboardCmd(flags))
	return cmd
}

func newManagerDashboardCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "dashboard",
		Short: "Pre-1:1 brief: direct reports, their check-ins, OKR health, and upcoming 1:1s",
		Long: `Joins users, userprofiles, check-ins, objectives, and oneonones from the
local SQLite store to produce a structured brief for each direct report.

Requires 'sync' to have been run first.`,
		Example: `  7geese-cli manager dashboard
  7geese-cli manager dashboard --json`,
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

			// Find direct reports from userprofile (manager field)
			// First, find the current user's profile ID
			profiles, err := directReportProfiles(db)
			if err != nil {
				return fmt.Errorf("querying direct reports: %w", err)
			}

			if len(profiles) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No direct reports found in local store. Run 'sync' first.")
				return nil
			}

			type reportBrief struct {
				UserID       string `json:"user_id"`
				Name         string `json:"name"`
				Email        string `json:"email"`
				LastCheckin  string `json:"last_checkin"`
				OKRHealth    string `json:"okr_health"`
				OpenOKRs     int    `json:"open_okrs"`
				Upcoming1on1 string `json:"upcoming_1on1"`
			}

			var briefs []reportBrief
			for _, p := range profiles {
				brief := reportBrief{
					UserID: p.userID,
					Name:   p.name,
					Email:  p.email,
				}

				// Last check-in
				var lastCheckin string
				row := db.DB().QueryRow(`
					SELECT json_extract(data,'$.created_at')
					FROM resources
					WHERE resource_type = 'checkins'
					  AND (json_extract(data,'$.creator') = ? OR json_extract(data,'$.creator') LIKE ?)
					ORDER BY json_extract(data,'$.created_at') DESC
					LIMIT 1
				`, p.userID, "%/"+p.userID+"/")
				_ = row.Scan(&lastCheckin)
				brief.LastCheckin = lastCheckin

				// OKR health
				openOKRs, health := userOKRHealth(db, p.userID)
				brief.OpenOKRs = openOKRs
				brief.OKRHealth = health

				// Next upcoming 1:1
				var upcoming1on1 string
				row = db.DB().QueryRow(`
					SELECT json_extract(data,'$.scheduled_date')
					FROM resources
					WHERE resource_type = 'oneonones'
					  AND json_extract(data,'$.status') = 'upcoming'
					  AND (json_extract(data,'$.target') = ? OR json_extract(data,'$.target') LIKE ?)
					ORDER BY json_extract(data,'$.scheduled_date') ASC
					LIMIT 1
				`, p.userID, "%/"+p.userID+"/")
				_ = row.Scan(&upcoming1on1)
				brief.Upcoming1on1 = upcoming1on1

				briefs = append(briefs, brief)
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(briefs)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Manager Dashboard — %d direct reports\n\n", len(briefs))
			for _, b := range briefs {
				fmt.Fprintf(w, "  %-30s  %-30s\n", b.Name, b.Email)
				fmt.Fprintf(w, "    Last check-in: %s\n", formatTimestamp(b.LastCheckin))
				fmt.Fprintf(w, "    OKRs: %d open, health: %s\n", b.OpenOKRs, b.OKRHealth)
				if b.Upcoming1on1 != "" {
					fmt.Fprintf(w, "    Next 1:1: %s\n", b.Upcoming1on1)
				}
				fmt.Fprintln(w)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

type userProfileRow struct {
	userID string
	name   string
	email  string
}

func directReportProfiles(db *store.Store) ([]userProfileRow, error) {
	// Find users where another profile's manager field points to them
	// We look for userprofile entries that have a manager URI
	rows, err := db.Query(`
		SELECT DISTINCT
			json_extract(r.data,'$.user') as user_uri,
			json_extract(u.data,'$.first_name') || ' ' || json_extract(u.data,'$.last_name') as full_name,
			json_extract(u.data,'$.email') as email
		FROM resources r
		JOIN resources u ON u.resource_type = 'user'
		WHERE r.resource_type = 'userprofile'
		  AND json_extract(r.data,'$.manager') IS NOT NULL
		  AND json_extract(r.data,'$.manager') != ''
		LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []userProfileRow
	for rows.Next() {
		var p userProfileRow
		if err := rows.Scan(&p.userID, &p.name, &p.email); err != nil {
			continue
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func userOKRHealth(db *store.Store, userID string) (int, string) {
	row := db.DB().QueryRow(`
		SELECT COUNT(*), AVG(CAST(json_extract(data,'$.progress') AS REAL))
		FROM resources
		WHERE resource_type = 'objectives'
		  AND COALESCE(json_extract(data,'$.closed'),0) = 0
		  AND (json_extract(data,'$.owner') = ? OR json_extract(data,'$.owner') LIKE ?)
	`, userID, "%/"+userID+"/")
	var count int
	var avgProgress float64
	_ = row.Scan(&count, &avgProgress)

	if count == 0 {
		return 0, "no_okrs"
	}
	health := "on_track"
	if avgProgress < 30 {
		health = "at_risk"
	} else if avgProgress < 70 {
		health = "progressing"
	}
	return count, health
}

func formatTimestamp(ts string) string {
	if ts == "" {
		return "never"
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, ts); err == nil {
			days := int(time.Since(t).Hours() / 24)
			if days == 0 {
				return "today"
			}
			if days == 1 {
				return "yesterday"
			}
			return fmt.Sprintf("%d days ago", days)
		}
	}
	return ts
}
