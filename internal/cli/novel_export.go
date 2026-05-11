// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

func newMeExportCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var output string
	var userFilter string
	var since string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a complete profile JSON: objectives, 1:1s, recognition, and reviews",
		Long: `Reads the local SQLite store and writes a single JSON file containing
all data for the specified user (defaults to you).

Sections included:
  profile      — name, email, position, hire date, manager
  objectives   — personal OKRs with nested key results and check-in history
  oneonones    — all 1:1 meetings with full Q&A from both participants
  recognition  — badges received and given
  reviews      — quarterly performance cycle records

Run 'sync' first to populate the store.`,
		Example: `  7geese-cli me export
  7geese-cli me export --output my-data.json
  7geese-cli me export | jq '.objectives'
  7geese-cli me export --user "Heather Moorhead"`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Guided flow: check auth, then data, prompting to fix each if missing.
			cfg, cfgErr := config.Load(flags.configPath)
			if cfgErr == nil && cfg.SevengeeseSession == "" {
				if !promptConfirm(cmd, flags, "Not authenticated. Run auth login now? [Y/n] ") {
					return fmt.Errorf("not authenticated — run '7geese-cli auth login' first")
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Reading session from browser...")
				browsers := resolveBrowserNames(false, false, false)
				session, csrf, _, authErr := extractKookyCookies(browsers)
				if authErr != nil {
					return fmt.Errorf("auth login failed: %w\nMake sure you are logged into app.7geese.com in Chrome or Firefox")
				}
				if err := cfg.SaveCookies(session, csrf); err != nil {
					return fmt.Errorf("saving session: %w", err)
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Authenticated.")
			}

			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}

			// Check if data exists; if not, offer to sync.
			needsSync := false
			db, openErr := store.OpenReadOnly(dbPath)
			if openErr != nil {
				needsSync = true
			} else {
				if _, err := resolveExportUser(db, userFilter); err != nil {
					needsSync = true
				}
				db.Close()
			}

			if needsSync {
				if !promptConfirm(cmd, flags, "No local data found. Run sync to fetch from 7Geese now? [Y/n] ") {
					return fmt.Errorf("no local data — run '7geese-cli sync' first")
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Syncing...")
				prevHumanFriendly := humanFriendly
				humanFriendly = true
				syncErr := doSync(context.Background(), flags, cmd.ErrOrStderr())
				humanFriendly = prevHumanFriendly
				if syncErr != nil {
					return fmt.Errorf("sync failed: %w", syncErr)
				}
				fmt.Fprintln(cmd.ErrOrStderr(), "Sync complete.")
			}

			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			profileID, err := resolveExportUser(db, userFilter)
			if err != nil {
				return err
			}

			out := exportOutput{
				ExportedAt: time.Now().UTC().Format(time.RFC3339),
				Me:         buildExportProfile(db, profileID, since),
			}
			for _, rid := range fetchDirectReportIDs(db, profileID) {
				out.DirectReports = append(out.DirectReports, buildExportProfile(db, rid, since))
			}

			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")

			if output != "" && output != "-" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("creating output file: %w", err)
				}
				defer f.Close()
				enc = json.NewEncoder(f)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported to %s\n", output)
				return nil
			}
			return enc.Encode(out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write JSON to a file (default: stdout — pipe to redirect)")
	cmd.Flags().StringVar(&userFilter, "user", "", "User to export: name, email, or numeric ID (default: you)")
	cmd.Flags().StringVar(&since, "since", "", "Inclusive start date (YYYY-MM-DD): exclude data older than this date")
	return cmd
}

// --- export types ---

type exportOutput struct {
	ExportedAt    string        `json:"exported_at" jsonschema:"description=ISO 8601 timestamp of when this export was generated"`
	Me            exportProfile `json:"me" jsonschema:"description=The authenticated user's full profile and history"`
	DirectReports []exportProfile `json:"direct_reports,omitempty" jsonschema:"description=Direct reports with the same data shape as me"`
}

type exportProfile struct {
	Profile      exportUser        `json:"profile" jsonschema:"description=User profile information"`
	Objectives   []exportObjective `json:"objectives" jsonschema:"description=OKRs where the user is an owner, stakeholder, or follower"`
	Oneonones    []exportMeeting   `json:"oneonones" jsonschema:"description=1:1 meeting history with full Q&A from both participants"`
	Recognitions exportRecognition `json:"recognitions" jsonschema:"description=Recognition badges sent and received"`
	Reviews      []exportReview    `json:"reviews" jsonschema:"description=Completed performance review cycles"`
}

type exportUser struct {
	ID       int    `json:"id" jsonschema:"description=7Geese user profile ID"`
	Name     string `json:"name" jsonschema:"description=Full name"`
	Email    string `json:"email,omitempty" jsonschema:"description=Work email address"`
	Position string `json:"position,omitempty" jsonschema:"description=Job title or position"`
	HireDate string `json:"hire_date,omitempty" jsonschema:"description=Date the user joined the organisation (YYYY-MM-DD)"`
	Manager  string `json:"manager,omitempty" jsonschema:"description=Full name of the user's direct manager"`
}

type exportObjective struct {
	ID              string          `json:"id" jsonschema:"description=Objective ID"`
	Name            string          `json:"name" jsonschema:"description=Objective title"`
	Description     string          `json:"description,omitempty" jsonschema:"description=Optional longer description of the objective"`
	Type            string          `json:"type" jsonschema:"description=Scope of the objective: personal, team, or org"`
	ParticipantType string          `json:"participant_type,omitempty" jsonschema:"description=The user's role on this objective: owner, stakeholder, or follower"`
	Progress        float64         `json:"progress" jsonschema:"description=Overall completion percentage (0–100)"`
	StartDate       string          `json:"start_date,omitempty" jsonschema:"description=Objective start date (YYYY-MM-DD)"`
	DueDate         string          `json:"due_date,omitempty" jsonschema:"description=Objective due date (YYYY-MM-DD)"`
	CompletedDate   string          `json:"completed_date,omitempty" jsonschema:"description=Date the objective was marked complete (YYYY-MM-DD)"`
	Closed          bool            `json:"closed" jsonschema:"description=Whether the objective is closed (completed or abandoned)"`
	KeyResults      []exportKR      `json:"key_results,omitempty" jsonschema:"description=Key results nested under this objective"`
	Checkins        []exportCheckin `json:"checkins,omitempty" jsonschema:"description=Most recent progress check-in"`
}

type exportKR struct {
	ID              string  `json:"id" jsonschema:"description=Key result ID"`
	Name            string  `json:"name" jsonschema:"description=Key result title"`
	Description     string  `json:"description,omitempty" jsonschema:"description=Optional longer description"`
	Progress        float64 `json:"progress" jsonschema:"description=Completion percentage (0–100)"`
	MeasurementType string  `json:"measurement_type,omitempty" jsonschema:"description=How progress is measured: percent, number, currency, or boolean"`
	CurrentValue    float64 `json:"current_value" jsonschema:"description=Current measured value"`
	TargetValue     float64 `json:"target_value" jsonschema:"description=Target value to reach 100% completion"`
}

type exportCheckin struct {
	ID               string  `json:"id" jsonschema:"description=Check-in ID"`
	Date             string  `json:"date" jsonschema:"description=Date the check-in was posted (ISO 8601)"`
	Message          string  `json:"message,omitempty" jsonschema:"description=Free-text update posted with this check-in"`
	ProgressSnapshot float64 `json:"progress_at_checkin,omitempty" jsonschema:"description=Progress percentage recorded at the time of this check-in"`
}

type exportMeeting struct {
	ID        string     `json:"id" jsonschema:"description=1:1 meeting ID"`
	Date      string     `json:"date" jsonschema:"description=Scheduled date and time of the meeting (ISO 8601)"`
	With      string     `json:"with" jsonschema:"description=Full name of the other participant"`
	Status    string     `json:"status" jsonschema:"description=Meeting status: completed or upcoming"`
	Questions []exportQA `json:"questions,omitempty" jsonschema:"description=Questions and answers from both participants"`
}

type exportQA struct {
	Question string         `json:"question" jsonschema:"description=The question text"`
	Hint     string         `json:"hint,omitempty" jsonschema:"description=Optional guidance or sub-prompt shown under the question"`
	Answers  []exportAnswer `json:"answers" jsonschema:"description=Answers submitted by each participant"`
}

type exportAnswer struct {
	Author string `json:"author" jsonschema:"description=Full name of the person who wrote this answer"`
	Text   string `json:"text" jsonschema:"description=The answer text"`
}

type exportRecognition struct {
	Received []exportBadge `json:"received" jsonschema:"description=Badges the user received from others"`
	Given    []exportBadge `json:"given" jsonschema:"description=Badges the user sent to others"`
}

type exportReview struct {
	ID           int                   `json:"id" jsonschema:"description=Performance review cycle ID"`
	Title        string                `json:"title" jsonschema:"description=Name of the review cycle (e.g. 'Q1 2026 Performance Review')"`
	Period       string                `json:"period" jsonschema:"description=Human-readable quarter label derived from the start date (e.g. Q1 2026)"`
	StartDate    string                `json:"start_date" jsonschema:"description=Review period start date (ISO 8601)"`
	EndDate      string                `json:"end_date" jsonschema:"description=Review period end date (ISO 8601)"`
	State        string                `json:"state" jsonschema:"description=Workflow state of the review cycle"`
	Position     string                `json:"position,omitempty" jsonschema:"description=The user's job title at the time of the review"`
	Manager      string                `json:"manager,omitempty" jsonschema:"description=Full name of the user's manager for this review"`
	Sections     []exportReviewSection `json:"sections,omitempty" jsonschema:"description=Review form sections containing questions and answers"`
	PeerFeedback *exportPeerReport     `json:"peer_feedback,omitempty" jsonschema:"description=Curated peer feedback report if one was published for this cycle"`
}

type exportReviewSection struct {
	Title string           `json:"title" jsonschema:"description=Section heading"`
	Items []exportReviewQA `json:"items" jsonschema:"description=Questions and answers within this section"`
}

type exportReviewQA struct {
	Question string `json:"question" jsonschema:"description=The review question text"`
	Employee string `json:"employee,omitempty" jsonschema:"description=The employee's answer"`
	Manager  string `json:"manager,omitempty" jsonschema:"description=The manager's response"`
}

type exportPeerReport struct {
	Title     string               `json:"title" jsonschema:"description=Title of the peer feedback request"`
	Published string               `json:"published,omitempty" jsonschema:"description=Date the curated report was published (ISO 8601)"`
	Questions []exportPeerQuestion `json:"questions,omitempty" jsonschema:"description=Questions with anonymised peer answers"`
}

type exportBadge struct {
	ID      string `json:"id" jsonschema:"description=Badge event ID"`
	Date    string `json:"date" jsonschema:"description=Date the recognition was given (ISO 8601)"`
	From    string `json:"from,omitempty" jsonschema:"description=Full name of the person who gave the badge"`
	To      string `json:"to,omitempty" jsonschema:"description=Full name of the person who received the badge"`
	Badge   string `json:"badge,omitempty" jsonschema:"description=Name of the badge awarded"`
	Message string `json:"message,omitempty" jsonschema:"description=Personal message included with the badge"`
}

type exportPeerQuestion struct {
	Question string   `json:"question" jsonschema:"description=The peer feedback question text"`
	Answers  []string `json:"answers,omitempty" jsonschema:"description=Anonymised answers submitted by peers"`
}

// --- builders ---

func resolveExportUser(db *store.Store, filter string) (int, error) {
	if filter != "" {
		// Try numeric ID first
		if id, err := strconv.Atoi(filter); err == nil {
			return id, nil
		}
		// Search by name or email
		row := db.DB().QueryRow(`
			SELECT CAST(json_extract(data,'$.id') AS INTEGER)
			FROM resources
			WHERE resource_type = 'userprofile'
			  AND (LOWER(json_extract(data,'$.user.email')) = LOWER(?)
			   OR LOWER(json_extract(data,'$.user.first_name') || ' ' || json_extract(data,'$.user.last_name')) = LOWER(?))
			LIMIT 1
		`, filter, filter)
		var id int
		if err := row.Scan(&id); err == nil && id > 0 {
			return id, nil
		}
		return 0, fmt.Errorf("user %q not found in local store (run 'sync' first)", filter)
	}

	// User ID is written to config by sync (from /status/ API call).
	cfg, err := config.Load("")
	if err != nil || cfg.SevengeeseUserID == 0 {
		return 0, fmt.Errorf("could not detect current user — run 'sync' first or pass --user")
	}
	return cfg.SevengeeseUserID, nil
}

func buildExportProfile(db *store.Store, profileID int, since string) exportProfile {
	return exportProfile{
		Profile:      fetchExportUser(db, profileID),
		Objectives:   fetchExportObjectives(db, profileID, since),
		Oneonones:    fetchExportOneonones(db, profileID, since),
		Recognitions: fetchExportRecognition(db, profileID, since),
		Reviews:      fetchExportReviews(db, profileID, since),
	}
}

func fetchDirectReportIDs(db *store.Store, profileID int) []int {
	managerURI := fmt.Sprintf("/api/v1/userprofile/%d/", profileID)
	rows, err := db.Query(`
		SELECT CAST(json_extract(data,'$.id') AS INTEGER)
		FROM resources
		WHERE resource_type = 'userprofile'
		  AND json_extract(data,'$.reports_to') = ?
		  AND CAST(json_extract(data,'$.id') AS INTEGER) != ?
	`, managerURI, profileID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func fetchExportUser(db *store.Store, profileID int) exportUser {
	row := db.DB().QueryRow(`
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER),
			json_extract(data,'$.user.first_name') || ' ' || json_extract(data,'$.user.last_name'),
			COALESCE(json_extract(data,'$.user.email'),''),
			COALESCE(json_extract(data,'$.position'),''),
			COALESCE(json_extract(data,'$.hire_date'),'')
		FROM resources
		WHERE resource_type = 'userprofile'
		  AND CAST(json_extract(data,'$.id') AS INTEGER) = ?
		LIMIT 1
	`, profileID)

	var u exportUser
	var fullName string
	_ = row.Scan(&u.ID, &fullName, &u.Email, &u.Position, &u.HireDate)
	u.Name = strings.TrimSpace(fullName)

	// Resolve manager name from reports_to URI
	managerRow := db.DB().QueryRow(`
		SELECT up.data
		FROM resources p
		JOIN resources up ON up.resource_type = 'userprofile'
		  AND json_extract(p.data,'$.reports_to') = json_extract(up.data,'$.resource_uri')
		WHERE p.resource_type = 'userprofile'
		  AND CAST(json_extract(p.data,'$.id') AS INTEGER) = ?
		LIMIT 1
	`, profileID)
	var managerData string
	if err := managerRow.Scan(&managerData); err == nil {
		var obj map[string]any
		if json.Unmarshal([]byte(managerData), &obj) == nil {
			if u2, ok := obj["user"].(map[string]any); ok {
				fn, _ := u2["first_name"].(string)
				ln, _ := u2["last_name"].(string)
				u.Manager = strings.TrimSpace(fn + " " + ln)
			}
		}
	}
	return u
}

func fetchExportObjectives(db *store.Store, profileID int, since string) []exportObjective {
	resourceType := fmt.Sprintf("user_objectives:%d", profileID)
	// When since is provided, include objectives whose period intersects [since, now]:
	//   - open objectives are always included (still active)
	//   - closed objectives where dueDatetime >= since (ended within or after the window)
	// When since is empty, all objectives (open + closed) are included.
	sinceClause := ""
	var args []any
	if since != "" {
		sinceClause = "AND (json_extract(data,'$.closed') = 0 OR json_extract(data,'$.dueDatetime') >= ?)"
		args = append(args, since)
	}
	rows, err := db.Query(`
		SELECT data FROM resources
		WHERE resource_type = ?
		  `+sinceClause+`
		ORDER BY json_extract(data,'$.closed') ASC, json_extract(data,'$.dueDatetime') DESC
	`, append([]any{resourceType}, args...)...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var objectives []exportObjective
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var node gqlObjectiveNode
		if err := json.Unmarshal([]byte(raw), &node); err != nil {
			continue
		}
		obj := exportObjective{
			ID:              strconv.Itoa(node.PK),
			Name:            node.Name,
			Description:     node.Description,
			Type:            objectiveTypeName(node.ObjectiveType),
			ParticipantType: participantTypeName(node.CurrentUserParticipantType),
			Progress:        node.Progress,
			StartDate:       node.StartDate,
			DueDate:         node.DueDatetime,
			CompletedDate:   node.CompletedDatetime,
			Closed:          node.Closed,
		}
		for _, kre := range node.KeyResults.Edges {
			kr := kre.Node
			obj.KeyResults = append(obj.KeyResults, exportKR{
				ID:              strconv.Itoa(kr.PK),
				Name:            kr.Name,
				Description:     kr.Description,
				Progress:        kr.Progress,
				MeasurementType: krMeasurementTypeName(kr.MeasurementType),
				CurrentValue:    kr.CurrentValue,
				TargetValue:     kr.TargetValue,
			})
		}
		if node.LastCheckin != nil {
			obj.Checkins = []exportCheckin{{
				ID:      strconv.Itoa(node.LastCheckin.PK),
				Date:    node.LastCheckin.Created,
				Message: node.LastCheckin.Message,
			}}
		}
		objectives = append(objectives, obj)
	}
	return objectives
}

func participantTypeName(t int) string {
	switch t {
	case 1:
		return "owner"
	case 2:
		return "stakeholder"
	case 3:
		return "follower"
	default:
		return ""
	}
}

func krMeasurementTypeName(t int) string {
	switch t {
	case 1:
		return "percent"
	case 2:
		return "number"
	case 3:
		return "currency"
	case 4:
		return "boolean"
	default:
		return "percent"
	}
}

func objectiveTypeName(t int) string {
	switch t {
	case 1:
		return "personal"
	case 2:
		return "team"
	case 3:
		return "org"
	default:
		return "personal"
	}
}

func fetchExportOneonones(db *store.Store, profileID int, since string) []exportMeeting {
	sinceClause := ""
	args := []any{profileID, profileID}
	if since != "" {
		sinceClause = "AND json_extract(data,'$.start_datetime') >= ?"
		args = append(args, since)
	}
	rows, err := db.Query(`
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER),
			COALESCE(json_extract(data,'$.start_datetime'),''),
			json_extract(data,'$.creator.user.first_name') || ' ' || json_extract(data,'$.creator.user.last_name'),
			json_extract(data,'$.target.user.first_name') || ' ' || json_extract(data,'$.target.user.last_name'),
			json_extract(data,'$.status')
		FROM resources
		WHERE resource_type = 'oneonones'
		  AND (CAST(json_extract(data,'$.creator.id') AS INTEGER) = ?
		    OR CAST(json_extract(data,'$.target.id') AS INTEGER) = ?)
		  `+sinceClause+`
		ORDER BY json_extract(data,'$.start_datetime') DESC
	`, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var meetings []exportMeeting
	for rows.Next() {
		var id int
		var date, creatorName, targetName string
		var statusRaw any
		if err := rows.Scan(&id, &date, &creatorName, &targetName, &statusRaw); err != nil {
			continue
		}
		status := "upcoming"
		switch v := statusRaw.(type) {
		case int64:
			if v == 1 {
				status = "completed"
			}
		case string:
			if v == "1" {
				status = "completed"
			}
		}

		// Determine "with" — the other person
		with := targetName
		if strings.TrimSpace(targetName) == "" || targetName == " " {
			with = creatorName
		}

		m := exportMeeting{
			ID:     strconv.Itoa(id),
			Date:   date,
			With:   strings.TrimSpace(with),
			Status: status,
		}

		// Attach full Q&A from finalized_oneonones
		raw, _ := db.Get("finalized_oneonones", strconv.Itoa(id))
		if raw != nil {
			var gql gqlMeeting
			if json.Unmarshal(raw, &gql) == nil {
				for _, qe := range gql.QuestionSet.Questions.Edges {
					q := qe.Node
					if q.Content == "" {
						continue
					}
					qa := exportQA{
						Question: q.Content,
						Hint:     q.Description,
					}
					for _, ne := range q.Notes.Edges {
						n := ne.Node
						if strings.TrimSpace(n.Content) == "" {
							continue
						}
						qa.Answers = append(qa.Answers, exportAnswer{
							Author: n.Creator.FullName,
							Text:   strings.TrimSpace(n.Content),
						})
					}
					m.Questions = append(m.Questions, qa)
				}
			}
		}
		meetings = append(meetings, m)
	}
	return meetings
}

func fetchExportRecognition(db *store.Store, profileID int, since string) exportRecognition {
	var rec exportRecognition
	profileURI := fmt.Sprintf("/api/v1/userprofile/%d/", profileID)

	sentSinceClause := ""
	sentArgs := []any{profileURI}
	if since != "" {
		sentSinceClause = "AND json_extract(data,'$.created') >= ?"
		sentArgs = append(sentArgs, since)
	}
	sentRows, err := db.Query(`
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER),
			COALESCE(json_extract(data,'$.created'),''),
			COALESCE(json_extract(data,'$.sender.user.first_name') || ' ' || json_extract(data,'$.sender.user.last_name'),''),
			COALESCE(json_extract(data,'$.recipient.user.first_name') || ' ' || json_extract(data,'$.recipient.user.last_name'),''),
			COALESCE(json_extract(data,'$.badge.name'),''),
			COALESCE(json_extract(data,'$.message'),'')
		FROM resources
		WHERE resource_type = 'recognitionbadges'
		  AND json_extract(data,'$.sender.resource_uri') = ?
		  `+sentSinceClause+`
		ORDER BY json_extract(data,'$.created') DESC
	`, sentArgs...)
	if err == nil {
		defer sentRows.Close()
		for sentRows.Next() {
			var id int
			var date, from, to, badge, message string
			if err := sentRows.Scan(&id, &date, &from, &to, &badge, &message); err != nil {
				continue
			}
			rec.Given = append(rec.Given, exportBadge{
				ID:      strconv.Itoa(id),
				Date:    date,
				From:    strings.TrimSpace(from),
				To:      strings.TrimSpace(to),
				Badge:   badge,
				Message: message,
			})
		}
	}

	recvSinceClause := ""
	var recvArgs []any
	if since != "" {
		recvSinceClause = "AND json_extract(data,'$.published') >= ?"
		recvArgs = append(recvArgs, since)
	}
	recvRows, err := db.Query(`
		SELECT
			json_extract(data,'$.id'),
			COALESCE(json_extract(data,'$.published'),''),
			COALESCE(json_extract(data,'$.actor.displayName'),''),
			COALESCE(json_extract(data,'$.target.displayName'),''),
			COALESCE(json_extract(data,'$.object.badgeName'),''),
			COALESCE(json_extract(data,'$.object.content'),'')
		FROM resources
		WHERE resource_type = 'recognition_received'
		  `+recvSinceClause+`
		ORDER BY json_extract(data,'$.published') DESC
	`, recvArgs...)
	if err == nil {
		defer recvRows.Close()
		for recvRows.Next() {
			var id, date, from, to, badge, message string
			if err := recvRows.Scan(&id, &date, &from, &to, &badge, &message); err != nil {
				continue
			}
			rec.Received = append(rec.Received, exportBadge{
				ID:      id,
				Date:    date,
				From:    strings.TrimSpace(from),
				To:      strings.TrimSpace(to),
				Badge:   badge,
				Message: message,
			})
		}
	}

	if rec.Received == nil {
		rec.Received = []exportBadge{}
	}
	if rec.Given == nil {
		rec.Given = []exportBadge{}
	}
	return rec
}

func fetchExportReviews(db *store.Store, profileID int, since string) []exportReview {
	resourceType := fmt.Sprintf("user_snapshots:%d", profileID)
	sinceClause := ""
	var args []any
	if since != "" {
		sinceClause = "AND json_extract(data,'$.endDate') >= ?"
		args = append(args, since)
	}
	rows, err := db.Query(`
		SELECT data FROM resources
		WHERE resource_type = ?
		  `+sinceClause+`
		ORDER BY json_extract(data,'$.startDate') DESC
	`, append([]any{resourceType}, args...)...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var reviews []exportReview
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var snap gqlSnapshotFull
		if err := json.Unmarshal([]byte(raw), &snap); err != nil {
			continue
		}

		r := exportReview{
			ID:        snap.PK,
			Title:     snap.SnapshotGroup.Title,
			Period:    snapshotPeriod(snap.StartDate),
			StartDate: snap.StartDate,
			EndDate:   snap.EndDate,
			State:     snapshotStateName(snap.WorkflowState),
			Position:  snap.Position,
		}
		if snap.Manager != nil {
			r.Manager = snap.Manager.FullName
		}

		// Build item pk -> title map from all sections.
		itemTitles := map[int]string{}
		for _, se := range snap.SnapshotGroup.Sections.Edges {
			for _, ie := range se.Node.Items.Edges {
				if ie.Node.PK != 0 && ie.Node.Title != "" {
					itemTitles[ie.Node.PK] = ie.Node.Title
				}
			}
		}

		// Build answer map: item pk -> {employee answer, manager answer}
		type answerPair struct{ employee, manager string }
		answers := map[int]*answerPair{}
		ensurePair := func(pk int) *answerPair {
			if answers[pk] == nil {
				answers[pk] = &answerPair{}
			}
			return answers[pk]
		}

		for _, ae := range snap.Answers.Edges {
			a := ae.Node
			pk := a.Item.PK
			if pk == 0 {
				continue
			}
			var text string
			switch {
			case a.Answer != "":
				text = stripHTMLTags(a.Answer)
			case a.RangeAnswer != nil:
				text = fmt.Sprintf("%.0f", *a.RangeAnswer)
			case len(a.Choices.Edges) > 0:
				var opts []string
				for _, ce := range a.Choices.Edges {
					opts = append(opts, ce.Node.Option.Title)
				}
				text = strings.Join(opts, ", ")
			}
			if text == "" {
				continue
			}
			pair := ensurePair(pk)
			// The employee/manager distinction: employee is forUser (profileID stored
			// in the snapshot), manager is the manager profile. We detect by comparing
			// the responder pk to the manager pk.
			isManager := snap.Manager != nil && a.Responder.PK != 0
			// Heuristic: if snapshotGroup has showManagerAnswers and this responder
			// is not the forUser, it's the manager. We use the manager pk via the
			// full snapshot's manager field — but we only have fullName not pk here.
			// Instead: employee answer goes first; if a second answer appears for the
			// same item from a different responder, it's the manager's.
			_ = isManager
			if pair.employee == "" {
				pair.employee = text
			} else {
				pair.manager = text
			}
		}

		// Assemble sections preserving section order and only including items with answers.
		for _, se := range snap.SnapshotGroup.Sections.Edges {
			sec := se.Node
			if sec.Title == "" && len(sec.Items.Edges) == 0 {
				continue
			}
			var items []exportReviewQA
			for _, ie := range sec.Items.Edges {
				item := ie.Node
				if item.PK == 0 {
					continue
				}
				pair := answers[item.PK]
				if pair == nil {
					continue
				}
				items = append(items, exportReviewQA{
					Question: item.Title,
					Employee: pair.employee,
					Manager:  pair.manager,
				})
			}
			if len(items) > 0 {
				r.Sections = append(r.Sections, exportReviewSection{
					Title: sec.Title,
					Items: items,
				})
			}
		}

		// Peer feedback curated report.
		if snap.PeerFeedbackRequest != nil && snap.PeerFeedbackRequest.CuratedReport != nil {
			cr := snap.PeerFeedbackRequest.CuratedReport
			peer := &exportPeerReport{
				Title:     snap.PeerFeedbackRequest.Title,
				Published: cr.PublishedDatetime,
			}
			for _, qe := range cr.SortedCuratedQuestions.Edges {
				q := qe.Node
				var questionTitle string
				// question is a polymorphic union — extract title field.
				var qMap map[string]any
				if json.Unmarshal(q.Question, &qMap) == nil {
					if t, ok := qMap["title"].(string); ok {
						questionTitle = t
					}
				}
				if questionTitle == "" {
					continue
				}
				pq := exportPeerQuestion{Question: questionTitle}
				for _, ae := range q.Answers.Edges {
					if txt := stripHTMLTags(ae.Node.Answer); txt != "" {
						pq.Answers = append(pq.Answers, txt)
					}
				}
				peer.Questions = append(peer.Questions, pq)
			}
			if len(peer.Questions) > 0 {
				r.PeerFeedback = peer
			}
		}

		reviews = append(reviews, r)
	}
	return reviews
}

func snapshotPeriod(startDate string) string {
	// startDate is like "2026-01-01" or "2026-01-01T00:00:00+00:00"
	if len(startDate) < 7 {
		return startDate
	}
	year := startDate[:4]
	month := startDate[5:7]
	switch month {
	case "01", "02", "03":
		return "Q1 " + year
	case "04", "05", "06":
		return "Q2 " + year
	case "07", "08", "09":
		return "Q3 " + year
	default:
		return "Q4 " + year
	}
}
