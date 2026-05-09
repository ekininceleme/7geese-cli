// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
  7geese-cli me export --output ekin-profile.json
  7geese-cli me export --user "Heather Moorhead"`,
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

			profileID, err := resolveExportUser(db, userFilter)
			if err != nil {
				return err
			}

			profile, err := buildExportProfile(db, profileID, since)
			if err != nil {
				return err
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
				if err := enc.Encode(profile); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported to %s\n", output)
				return nil
			}
			return enc.Encode(profile)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: stdout)")
	cmd.Flags().StringVar(&userFilter, "user", "", "User to export: name, email, or numeric ID (default: you)")
	cmd.Flags().StringVar(&since, "since", "", "Inclusive start date (YYYY-MM-DD): exclude data older than this date")
	return cmd
}

// --- export types ---

type exportProfile struct {
	ExportedAt   string            `json:"exported_at"`
	Profile      exportUser        `json:"profile"`
	Objectives   []exportObjective `json:"objectives"`
	Oneonones    []exportMeeting   `json:"oneonones"`
	Recognitions exportRecognition `json:"recognitions"`
	Reviews      []exportReview    `json:"reviews"`
}

type exportUser struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Position string `json:"position,omitempty"`
	HireDate string `json:"hire_date,omitempty"`
	Manager  string `json:"manager,omitempty"`
}

type exportObjective struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Type            string          `json:"type"`
	ParticipantType string          `json:"participant_type,omitempty"`
	Progress        float64         `json:"progress"`
	StartDate       string          `json:"start_date,omitempty"`
	DueDate         string          `json:"due_date,omitempty"`
	CompletedDate   string          `json:"completed_date,omitempty"`
	Closed          bool            `json:"closed"`
	KeyResults      []exportKR      `json:"key_results,omitempty"`
	Checkins        []exportCheckin `json:"checkins,omitempty"`
}

type exportKR struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	Progress        float64 `json:"progress"`
	MeasurementType string  `json:"measurement_type,omitempty"`
	CurrentValue    float64 `json:"current_value"`
	TargetValue     float64 `json:"target_value"`
}

type exportCheckin struct {
	ID               string  `json:"id"`
	Date             string  `json:"date"`
	Message          string  `json:"message,omitempty"`
	ProgressSnapshot float64 `json:"progress_at_checkin,omitempty"`
}

type exportMeeting struct {
	ID        string      `json:"id"`
	Date      string      `json:"date"`
	With      string      `json:"with"`
	Status    string      `json:"status"`
	Questions []exportQA  `json:"questions,omitempty"`
}

type exportQA struct {
	Question string         `json:"question"`
	Hint     string         `json:"hint,omitempty"`
	Answers  []exportAnswer `json:"answers"`
}

type exportAnswer struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

type exportRecognition struct {
	Received []exportBadge `json:"received"`
	Given    []exportBadge `json:"given"`
}

type exportReview struct {
	ID           int                   `json:"id"`
	Title        string                `json:"title"`
	Period       string                `json:"period"`
	StartDate    string                `json:"start_date"`
	EndDate      string                `json:"end_date"`
	State        string                `json:"state"`
	Position     string                `json:"position,omitempty"`
	Manager      string                `json:"manager,omitempty"`
	Sections     []exportReviewSection `json:"sections,omitempty"`
	PeerFeedback *exportPeerReport     `json:"peer_feedback,omitempty"`
}

type exportReviewSection struct {
	Title string           `json:"title"`
	Items []exportReviewQA `json:"items"`
}

type exportReviewQA struct {
	Question string `json:"question"`
	Employee string `json:"employee,omitempty"`
	Manager  string `json:"manager,omitempty"`
}

type exportPeerReport struct {
	Title     string               `json:"title"`
	Published string               `json:"published,omitempty"`
	Questions []exportPeerQuestion `json:"questions,omitempty"`
}

type exportPeerQuestion struct {
	Question string   `json:"question"`
	Answers  []string `json:"answers,omitempty"`
}

type exportBadge struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Badge   string `json:"badge,omitempty"`
	Message string `json:"message,omitempty"`
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

	// Default: detect current user from performance cycles (target is always the authed user)
	row := db.DB().QueryRow(`
		SELECT CAST(json_extract(data,'$.target.id') AS INTEGER)
		FROM resources WHERE resource_type = 'performancecycles'
		LIMIT 1
	`)
	var id int
	if err := row.Scan(&id); err == nil && id > 0 {
		return id, nil
	}

	// Fall back: most frequent participant in oneonones
	row = db.DB().QueryRow(`
		SELECT json_extract(data,'$.target.id'), COUNT(*) as c
		FROM resources WHERE resource_type = 'oneonones'
		GROUP BY json_extract(data,'$.target.id')
		ORDER BY c DESC LIMIT 1
	`)
	if err := row.Scan(&id); err == nil && id > 0 {
		return id, nil
	}
	return 0, fmt.Errorf("could not detect current user — run 'sync' first or pass --user")
}

func buildExportProfile(db *store.Store, profileID int, since string) (*exportProfile, error) {
	profile := &exportProfile{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	profile.Profile = fetchExportUser(db, profileID)
	profile.Objectives = fetchExportObjectives(db, profileID, since)
	profile.Oneonones = fetchExportOneonones(db, profileID, since)
	profile.Recognitions = fetchExportRecognition(db, profileID, since)
	profile.Reviews = fetchExportReviews(db, profileID, since)

	return profile, nil
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

func fetchExportObjectives(db *store.Store, _ int, since string) []exportObjective {
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
		WHERE resource_type = 'user_objectives'
		  `+sinceClause+`
		ORDER BY json_extract(data,'$.closed') ASC, json_extract(data,'$.dueDatetime') DESC
	`, args...)
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

func fetchExportReviews(db *store.Store, _ int, since string) []exportReview {
	sinceClause := ""
	var args []any
	if since != "" {
		sinceClause = "AND json_extract(data,'$.endDate') >= ?"
		args = append(args, since)
	}
	rows, err := db.Query(`
		SELECT data FROM resources
		WHERE resource_type = 'user_snapshots'
		  `+sinceClause+`
		ORDER BY json_extract(data,'$.startDate') DESC
	`, args...)
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
