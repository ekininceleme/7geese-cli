// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

type gqlSyncWG = gosync.WaitGroup

func newOneononesHistoryCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var with string
	var limit int
	var showNotes bool

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show past 1:1 meetings with their notes",
		Long: `Queries the local SQLite store (populated by 'sync') and shows all
completed 1:1 meetings, with the notes from each meeting printed inline.

Use --with to filter by a specific person (name, email, or user ID fragment).
Use --limit to cap the number of meetings shown (default 20).

The note content field varies by 7Geese configuration — the CLI tries
'note', 'body', 'content', and 'text' automatically.

Run 'sync' first to populate the local store.`,
		Example: `  7geese-cli oneonones history
  7geese-cli oneonones history --with alice
  7geese-cli oneonones history --with alice@company.com --json
  7geese-cli oneonones history --limit 5`,
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

			// Fetch meetings from store
			meetings, err := fetchMeetings(db, with, limit)
			if err != nil {
				return fmt.Errorf("querying meetings: %w", err)
			}

			if len(meetings) == 0 {
				msg := "No completed 1:1 meetings found in local store."
				if with != "" {
					msg = fmt.Sprintf("No completed 1:1 meetings found with %q.", with)
				}
				fmt.Fprintln(cmd.OutOrStdout(), msg+" Run 'sync' first.")
				return nil
			}

			if flags.asJSON {
				// For JSON output, attach finalized data to each meeting
				type meetingWithGQL struct {
					oneoononeMeeting
					Full *gqlMeeting `json:"full,omitempty"`
				}
				var out []meetingWithGQL
				for _, m := range meetings {
					row := meetingWithGQL{oneoononeMeeting: m}
					if raw, _ := db.Get("finalized_oneonones", m.ID); raw != nil {
						var gql gqlMeeting
						if json.Unmarshal(raw, &gql) == nil {
							row.Full = &gql
						}
					}
					out = append(out, row)
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "1:1 History (%d meetings)\n", len(meetings))
			if with != "" {
				fmt.Fprintf(w, "Filtered to: %s\n", with)
			}
			fmt.Fprintln(w)

			for _, m := range meetings {
				fmt.Fprintf(w, "──────────────────────────────────────────\n")
				fmt.Fprintf(w, "Date:   %s\n", formatDate(m.ScheduledDate))
				if m.Target != "" {
					fmt.Fprintf(w, "With:   %s\n", m.Target)
				}
				if m.Status != "" {
					fmt.Fprintf(w, "Status: %s\n", m.Status)
				}

				raw, _ := db.Get("finalized_oneonones", m.ID)
				if raw != nil {
					var gql gqlMeeting
					if json.Unmarshal(raw, &gql) == nil {
						renderGQLNotesInline(w, &gql)
						fmt.Fprintln(w)
						continue
					}
				}
				fmt.Fprintln(w, "(no notes — run 'sync' to download)")
				fmt.Fprintln(w)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVar(&with, "with", "", "Filter by person: name, email, or user ID fragment")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of meetings to show")
	cmd.Flags().BoolVar(&showNotes, "notes", true, "Include note content (default true)")
	return cmd
}

func newOneononesNotesCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "notes <meeting-id>",
		Short: "Show all notes for a specific 1:1 meeting, including manager replies",
		Long: `Shows full notes for a meeting, including both participants' replies per question.
Data is read from the local store — run 'sync' first to download it.`,
		Args: cobra.ExactArgs(1),
		Example: `  7geese-cli oneonones notes 12345
  7geese-cli oneonones notes 12345 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			meetingID := args[0]
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database (run 'sync' first): %w", err)
			}
			defer db.Close()

			raw, err := db.Get("finalized_oneonones", meetingID)
			if err != nil || raw == nil {
				return fmt.Errorf("no data for meeting %s — run 'sync' first", meetingID)
			}

			var m gqlMeeting
			if err := json.Unmarshal(raw, &m); err != nil {
				return fmt.Errorf("parsing stored meeting data: %w", err)
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(m)
			}
			return renderGraphQLMeeting(cmd, &m)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

func newOneononesSearchCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search through 1:1 note content",
		Args:  cobra.ExactArgs(1),
		Long: `Full-text search across all synced 1:1 note content using SQLite FTS5.
Matches are returned with the meeting date and participant.

Run 'sync' first to populate the local store.`,
		Example: `  7geese-cli oneonones search "career goals"
  7geese-cli oneonones search "promotion" --limit 5 --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := store.OpenReadOnly(dbPath)
			if err != nil {
				return fmt.Errorf("opening local database (run 'sync' first): %w", err)
			}
			defer db.Close()

			// LIKE search across note content, joined to the meeting for date/participant
			rows, err := db.Query(`
				SELECT
					n.id,
					n.data,
					COALESCE(json_extract(m.data,'$.start_datetime'),'') as meeting_date,
					json_extract(m.data,'$.creator.user.first_name') || ' ' || json_extract(m.data,'$.creator.user.last_name') as meeting_target
				FROM resources n
				LEFT JOIN resources m ON m.resource_type = 'oneonones'
					AND json_extract(n.data,'$.oneonone') = '/api/v1/oneonones/' || CAST(json_extract(m.data,'$.id') AS INTEGER) || '/'
				WHERE n.resource_type = 'oneononenotes'
				  AND typeof(json_extract(n.data,'$.note')) = 'text'
				  AND length(json_extract(n.data,'$.note')) > 3
				  AND LOWER(json_extract(n.data,'$.note')) LIKE LOWER(?)
				ORDER BY json_extract(m.data,'$.start_datetime') DESC
				LIMIT ?
			`, "%"+query+"%", limit)
			if err != nil {
				return fmt.Errorf("searching notes: %w", err)
			}
			defer rows.Close()

			type searchResult struct {
				NoteID      string `json:"note_id"`
				MeetingDate string `json:"meeting_date"`
				With        string `json:"with"`
				Excerpt     string `json:"excerpt"`
				RawNote     json.RawMessage `json:"raw,omitempty"`
			}

			var results []searchResult
			for rows.Next() {
				var noteID, rawData, meetingDate, meetingTarget string
				if err := rows.Scan(&noteID, &rawData, &meetingDate, &meetingTarget); err != nil {
					continue
				}
				body := extractNoteBody(rawData)
				excerpt := makeExcerpt(body, query, 200)
				r := searchResult{
					NoteID:      noteID,
					MeetingDate: formatDate(meetingDate),
					With:        meetingTarget,
					Excerpt:     excerpt,
				}
				if flags.asJSON {
					r.RawNote = json.RawMessage(rawData)
				}
				results = append(results, r)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("iterating results: %w", err)
			}

			if len(results) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No 1:1 notes matching %q found. Run 'sync' first.\n", query)
				return nil
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d note(s) matching %q\n\n", len(results), query)
			for _, r := range results {
				fmt.Fprintf(w, "  [%s]", r.MeetingDate)
				if r.With != "" {
					fmt.Fprintf(w, " with %s", r.With)
				}
				fmt.Fprintln(w)
				fmt.Fprintf(w, "  …%s…\n\n", r.Excerpt)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of results")
	return cmd
}

// --- data types ---

type oneononeNote struct {
	ID          string `json:"id"`
	Question    string `json:"question,omitempty"`
	Description string `json:"description,omitempty"`
	Body        string `json:"body"`
	Creator     string `json:"creator"`
	CreatedAt   string `json:"created_at"`
	MeetingID   string `json:"oneonone_id"`
}

type oneoononeMeeting struct {
	ID            string         `json:"id"`
	Target        string         `json:"target"`
	Status        string         `json:"status"`
	ScheduledDate string         `json:"scheduled_date"`
	Notes         []oneononeNote `json:"notes"`
}

// --- helpers ---

func fetchMeetings(db *store.Store, withFilter string, limit int) ([]oneoononeMeeting, error) {
	// target and creator are nested objects: target.user.first_name, target.user.last_name
	// start_datetime is the actual meeting date field
	query := `
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER) as meeting_id,
			data,
			json_extract(data,'$.start_datetime') as start_dt,
			json_extract(data,'$.creator.user.first_name') || ' ' || json_extract(data,'$.creator.user.last_name') as creator_name,
			json_extract(data,'$.target.user.first_name') || ' ' || json_extract(data,'$.target.user.last_name') as target_name,
			json_extract(data,'$.status') as status
		FROM resources
		WHERE resource_type = 'oneonones'
	`
	args := []any{}

	if withFilter != "" {
		query += ` AND (
			LOWER(json_extract(data,'$.creator.user.first_name') || ' ' || json_extract(data,'$.creator.user.last_name')) LIKE LOWER(?)
			OR LOWER(json_extract(data,'$.target.user.first_name') || ' ' || json_extract(data,'$.target.user.last_name')) LIKE LOWER(?)
			OR LOWER(json_extract(data,'$.creator.user.email')) LIKE LOWER(?)
			OR LOWER(json_extract(data,'$.target.user.email')) LIKE LOWER(?)
		)`
		like := "%" + withFilter + "%"
		args = append(args, like, like, like, like)
	}

	query += ` ORDER BY json_extract(data,'$.start_datetime') DESC`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var meetings []oneoononeMeeting
	for rows.Next() {
		var id, rawData, startDT, creatorName, targetName, status string
		if err := rows.Scan(&id, &rawData, &startDT, &creatorName, &targetName, &status); err != nil {
			continue
		}
		humanStatus := status
		if status == "1" {
			humanStatus = "completed"
		} else if status == "0" {
			humanStatus = "upcoming"
		}
		m := oneoononeMeeting{
			ID:            id,
			ScheduledDate: startDT,
			Status:        humanStatus,
		}
		// Display "Creator with Target" as the participants line
		switch {
		case creatorName != " " && targetName != " ":
			m.Target = creatorName + " with " + targetName
		case creatorName != " ":
			m.Target = creatorName
		case targetName != " ":
			m.Target = targetName
		}
		meetings = append(meetings, m)
	}
	return meetings, rows.Err()
}

func fetchNotesForMeeting(db *store.Store, meetingID string) ([]oneononeNote, error) {
	uri := "/api/v1/oneonones/" + meetingID + "/"
	rows, err := db.Query(`
		SELECT id, data,
			COALESCE(json_extract(data,'$.question'),'') as question,
			COALESCE(json_extract(data,'$.description'),'') as description
		FROM resources
		WHERE resource_type = 'oneononenotes'
		  AND json_extract(data, '$.oneonone') = ?
		  AND typeof(json_extract(data,'$.note')) = 'text'
		  AND (
		    length(COALESCE(json_extract(data,'$.note'),'')) > 3
		    OR json_extract(data,'$.question') != ''
		  )
		ORDER BY json_extract(data,'$.created') ASC
	`, uri)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notes []oneononeNote
	for rows.Next() {
		var id, rawData, question, description string
		if err := rows.Scan(&id, &rawData, &question, &description); err != nil {
			continue
		}
		n := oneononeNote{ID: id, MeetingID: meetingID, Question: question, Description: description}
		n.Body = extractNoteBody(rawData)
		if n.Body == "" {
			continue
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(rawData), &obj); err == nil {
			if v, ok := obj["creator"].(string); ok && v != "" {
				n.Creator = resolveUserProfileName(db, v)
			}
			for _, f := range []string{"created", "modified"} {
				if v, ok := obj[f].(string); ok && v != "" {
					n.CreatedAt = v
					break
				}
			}
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// extractNoteBody tries common field names for note text content.
// 7Geese field name is unknown until first sync; we try all plausible names.
func extractNoteBody(rawData string) string {
	var obj map[string]any
	if err := json.Unmarshal([]byte(rawData), &obj); err != nil {
		return ""
	}
	for _, field := range []string{"note", "body", "content", "text", "message", "description"} {
		if v, ok := obj[field].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// makeExcerpt returns a short window of text centred around the first match.
func makeExcerpt(body, query string, maxLen int) string {
	lower := strings.ToLower(body)
	idx := strings.Index(lower, strings.ToLower(query))
	if idx < 0 {
		if len(body) > maxLen {
			return body[:maxLen]
		}
		return body
	}
	start := idx - 60
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 60
	if end > len(body) {
		end = len(body)
	}
	return body[start:end]
}

// resolveUserProfileName turns a userprofile URI into "First Last" by joining
// the userprofile record (which embeds the user sub-object) in the local store.
func resolveUserProfileName(db *store.Store, profileURI string) string {
	// Extract numeric ID from /api/v1/userprofile/<id>/
	parts := strings.Split(strings.TrimRight(profileURI, "/"), "/")
	profileID := parts[len(parts)-1]
	if profileID == "" {
		return profileURI
	}

	var firstName, lastName string
	row := db.DB().QueryRow(`
		SELECT
			json_extract(data,'$.user.first_name'),
			json_extract(data,'$.user.last_name')
		FROM resources
		WHERE resource_type = 'userprofile'
		  AND CAST(id AS INTEGER) = ?
		LIMIT 1
	`, profileID)
	if err := row.Scan(&firstName, &lastName); err != nil {
		return profileID // fallback to bare ID
	}
	name := strings.TrimSpace(firstName + " " + lastName)
	if name == "" {
		return profileID
	}
	return name
}

// --- GraphQL support ---

const gqlFinalizedQuery = `query getFinalizedOneonone($oneononeId: Int!) {
  oneonone(pk: $oneononeId) {
    name
    facilitator { pk fullName __typename }
    participant  { pk fullName __typename }
    questionSet {
      questions: typedQuestions(first: 200) {
        edges {
          node {
            ... on OneOnOneQuestionInterface {
              pk
              content: title
              description
              notes(first: 10) {
                edges {
                  node {
                    ... on OneOnOneNoteInterface {
                      pk
                      creator { pk fullName __typename }
                      content: textAnswer
                      __typename
                    }
                    __typename
                  }
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    comments(first: 300) {
      edges {
        node {
          pk
          content
          creator { pk fullName __typename }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}`

type gqlUser struct {
	PK       int    `json:"pk"`
	FullName string `json:"fullName"`
}

type gqlNoteNode struct {
	PK      int     `json:"pk"`
	Creator gqlUser `json:"creator"`
	Content string  `json:"content"`
}

type gqlQuestionNode struct {
	PK          int    `json:"pk"`
	Content     string `json:"content"`
	Description string `json:"description"`
	Notes       struct {
		Edges []struct {
			Node gqlNoteNode `json:"node"`
		} `json:"edges"`
	} `json:"notes"`
}

type gqlComment struct {
	PK      int     `json:"pk"`
	Content string  `json:"content"`
	Creator gqlUser `json:"creator"`
}

type gqlMeeting struct {
	Name        string  `json:"name"`
	Facilitator gqlUser `json:"facilitator"`
	Participant gqlUser `json:"participant"`
	QuestionSet struct {
		Questions struct {
			Edges []struct {
				Node gqlQuestionNode `json:"node"`
			} `json:"edges"`
		} `json:"questions"`
	} `json:"questionSet"`
	Comments struct {
		Edges []struct {
			Node gqlComment `json:"node"`
		} `json:"edges"`
	} `json:"comments"`
}

func fetchMeetingFromGraphQL(flags *rootFlags, meetingID string) (*gqlMeeting, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return nil, fmt.Errorf("no auth")
	}

	id, err := strconv.Atoi(meetingID)
	if err != nil {
		return nil, fmt.Errorf("invalid meeting ID: %w", err)
	}

	body, _ := json.Marshal(map[string]any{
		"operationName": "getFinalizedOneonone",
		"variables":     map[string]any{"oneononeId": id},
		"query":         gqlFinalizedQuery,
	})

	req, err := http.NewRequest("POST", cfg.BaseURL+"/graphql?opname=getFinalizedOneonone", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	referer := cfg.BaseURL + "/oneonone/" + meetingID
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
	req.Header.Set("X-CSRFToken", cfg.SevengeeseCSRF)
	req.Header.Set("Referer", referer)
	req.Header.Set("X-HREF", referer)
	req.Header.Set("Origin", cfg.BaseURL)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Data struct {
			Oneonone gqlMeeting `json:"oneonone"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	m := envelope.Data.Oneonone
	return &m, nil
}

// syncFinalizedMeetings fetches both participants' notes via GraphQL for every
// completed meeting and stores the result in the local store as
// "finalized_oneonones". Called by the sync command after the main REST sync.
// Already-stored meetings are skipped unless force is true.
// Calls are parallelised with up to 8 workers to avoid sequential round-trip cost.
func syncFinalizedMeetings(flags *rootFlags, db *store.Store, force bool) (int, error) {
	rows, err := db.Query(`
		SELECT CAST(json_extract(data,'$.id') AS INTEGER)
		FROM resources
		WHERE resource_type = 'oneonones'
		  AND json_extract(data,'$.status') = 1
		ORDER BY json_extract(data,'$.start_datetime') DESC
	`)
	if err != nil {
		return 0, fmt.Errorf("listing meetings: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	_ = rows.Close()

	// Filter out already-synced meetings before spinning up workers.
	if !force {
		var pending []string
		for _, id := range ids {
			existing, _ := db.Get("finalized_oneonones", id)
			if existing == nil {
				pending = append(pending, id)
			}
		}
		ids = pending
	}

	if len(ids) == 0 {
		return 0, nil
	}

	const workers = 8
	work := make(chan string, len(ids))
	for _, id := range ids {
		work <- id
	}
	close(work)

	type result struct {
		id   string
		data json.RawMessage
	}
	results := make(chan result, len(ids))

	var wg gqlSyncWG
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				m, err := fetchMeetingFromGraphQL(flags, id)
				if err != nil || m == nil {
					continue
				}
				d, err := json.Marshal(m)
				if err != nil {
					continue
				}
				results <- result{id: id, data: d}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	synced := 0
	for r := range results {
		if err := db.Upsert("finalized_oneonones", r.id, json.RawMessage(r.data)); err != nil {
			continue
		}
		synced++
	}
	return synced, nil
}

func renderGQLNotesInline(w interface{ Write([]byte) (int, error) }, m *gqlMeeting) {
	hasContent := false
	for _, qe := range m.QuestionSet.Questions.Edges {
		q := qe.Node
		if q.Content == "" {
			continue
		}
		hasNotes := false
		for _, ne := range q.Notes.Edges {
			if strings.TrimSpace(ne.Node.Content) != "" {
				hasNotes = true
				break
			}
		}
		if !hasNotes {
			continue
		}
		hasContent = true
		fmt.Fprintf(w, "\n  Q: %s\n", q.Content)
		if q.Description != "" {
			fmt.Fprintf(w, "     (%s)\n", q.Description)
		}
		for _, ne := range q.Notes.Edges {
			n := ne.Node
			if strings.TrimSpace(n.Content) == "" {
				continue
			}
			fmt.Fprintf(w, "  A [%s]:\n", n.Creator.FullName)
			for _, line := range strings.Split(strings.TrimSpace(n.Content), "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
	}
	if !hasContent {
		fmt.Fprintf(w, "(no notes)\n")
	}
	for _, ce := range m.Comments.Edges {
		c := ce.Node
		if strings.TrimSpace(c.Content) == "" {
			continue
		}
		fmt.Fprintf(w, "  [comment — %s]: %s\n", c.Creator.FullName, strings.TrimSpace(c.Content))
	}
}

func renderGraphQLMeeting(cmd *cobra.Command, m *gqlMeeting) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%s\n", m.Name)
	fmt.Fprintf(w, "%s (facilitator) · %s (participant)\n\n", m.Facilitator.FullName, m.Participant.FullName)

	for _, qe := range m.QuestionSet.Questions.Edges {
		q := qe.Node
		if q.Content == "" {
			continue
		}
		fmt.Fprintf(w, "Q: %s\n", q.Content)
		if q.Description != "" {
			fmt.Fprintf(w, "   (%s)\n", q.Description)
		}
		for _, ne := range q.Notes.Edges {
			n := ne.Node
			if strings.TrimSpace(n.Content) == "" {
				continue
			}
			fmt.Fprintf(w, "  A [%s]:\n", n.Creator.FullName)
			for _, line := range strings.Split(strings.TrimSpace(n.Content), "\n") {
				fmt.Fprintf(w, "    %s\n", line)
			}
		}
		fmt.Fprintln(w)
	}

	// Show meeting comments if any
	comments := m.Comments.Edges
	if len(comments) > 0 {
		fmt.Fprintf(w, "── Comments ──\n")
		for _, ce := range comments {
			c := ce.Node
			if strings.TrimSpace(c.Content) == "" {
				continue
			}
			fmt.Fprintf(w, "  [%s]: %s\n", c.Creator.FullName, strings.TrimSpace(c.Content))
		}
		fmt.Fprintln(w)
	}
	return nil
}

func formatDate(ts string) string {
	if ts == "" {
		return "unknown date"
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.Format("2 Jan 2006")
		}
	}
	return ts
}
