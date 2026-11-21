// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
	"github.com/spf13/cobra"
)

const exportDateLayout = "2006-01-02"

var browserLogin = func() (string, string, error) {
	session, csrf, _, err := extractKookyCookies(resolveBrowserNames(false, false, false))
	return session, csrf, err
}

func newMeExportCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	var output string
	var userFilter string
	var since string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export a complete profile JSON: objectives, 1:1s, recognition, reviews, check-ins and notifications",
		Long: `Reads the local SQLite store and writes one JSON document with all data
for the specified user (the default is you).

Sections:
  profile        name, email, position, hire date, manager
  objectives     OKRs with key results and the full check-in history
  reviews        performance reviews in all workflow states
  feedback       feedback requests about you with the responses
  oneonones      1:1 meetings with the questions and answers of the two participants
  recognitions   badges received and given
  checkins       check-ins that you posted
  notifications  your notification feed
  direct_reports profile, objectives and reviews of each direct report

The sections feedback, oneonones, recognitions, checkins and notifications are
only available for you, because 7Geese gives them only for the session user.
The data of other users is limited to your direct reports.

The field synced_at shows the age of the local data. Run 'sync' to refresh it.
If the local data is not complete, the command asks to run 'sync'.`,
		Example: `  7geese-cli me export
  7geese-cli me export --output my-data.json
  7geese-cli me export | jq '.me.objectives'
  7geese-cli me export --since 2026-01-01
  7geese-cli me export --user 123456`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if since != "" {
				if _, err := time.Parse(exportDateLayout, since); err != nil {
					return fmt.Errorf("invalid --since value %q: use the format YYYY-MM-DD", since)
				}
			}
			if dbPath == "" {
				dbPath = defaultDBPath("7geese-cli")
			}
			db, err := openSyncedDatabase(cmd, flags, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()

			out, err := buildExport(db, userFilter, since, time.Now())
			if err != nil {
				return err
			}

			if output != "" && output != "-" {
				f, err := os.Create(output)
				if err != nil {
					return fmt.Errorf("creating output file: %w", err)
				}
				enc := json.NewEncoder(f)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					f.Close()
					return fmt.Errorf("writing output file: %w", err)
				}
				if err := f.Close(); err != nil {
					return fmt.Errorf("closing output file: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Exported to %s\n", output)
				return nil
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write JSON to a file (default: stdout)")
	cmd.Flags().StringVar(&userFilter, "user", "", "User to export: name, email, or numeric ID (default: you)")
	cmd.Flags().StringVar(&since, "since", "", "Start date (YYYY-MM-DD, 00:00 local time): exclude data older than this date")
	return cmd
}

func requiredDatasets(sessionUser int) []store.DatasetKey {
	return []store.DatasetKey{
		sessionDataset(resourceUserProfile),
		sessionDataset(resourceOneonones),
		sessionDataset(resourceFinalizedOneonones),
		sessionDataset(resourceRecognitionGiven),
		sessionDataset(resourceRecognitionGot),
		sessionDataset(resourceCheckins),
		sessionDataset(resourceNotifications),
		userDataset(resourceUserObjectives, sessionUser),
		userDataset(resourceUserSnapshots, sessionUser),
		sessionDataset(resourceFeedbackRequests),
	}
}

func missingSyncReason(db *store.Store) (string, error) {
	sessionUser, found, err := storedSessionUser(db)
	if err != nil {
		return "", err
	}
	if !found {
		return "no sync was completed for this database", nil
	}
	for _, key := range requiredDatasets(sessionUser) {
		_, synced, err := db.DatasetSyncedAt(key)
		if err != nil {
			return "", err
		}
		if !synced {
			return fmt.Sprintf("the dataset %s is not synced", key), nil
		}
	}
	return "", nil
}

func ensureSession(cmd *cobra.Command, flags *rootFlags) error {
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if cfg.SevengeeseSession != "" {
		return nil
	}
	if !promptConfirm(cmd, flags, "Not authenticated. Run auth login now? [Y/n] ") {
		return errors.New("not authenticated: run '7geese-cli auth login' first")
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Reading session from browser...")
	session, csrf, err := browserLogin()
	if err != nil {
		return fmt.Errorf("auth login failed: %w\nMake sure you are logged into app.7geese.com in Chrome or Firefox", err)
	}
	if err := cfg.SaveCookies(session, csrf); err != nil {
		return fmt.Errorf("saving session: %w", err)
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Authenticated.")
	return nil
}

func openSyncedDatabase(cmd *cobra.Command, flags *rootFlags, dbPath string) (*store.Store, error) {
	var db *store.Store
	reason := "the local database does not exist"
	_, statErr := os.Stat(dbPath)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("checking local database: %w", statErr)
	}
	if statErr == nil {
		opened, err := store.OpenWithContext(cmd.Context(), dbPath)
		if err != nil {
			return nil, fmt.Errorf("opening local database: %w", err)
		}
		reason, err = missingSyncReason(opened)
		if err != nil {
			opened.Close()
			return nil, err
		}
		if reason == "" {
			return opened, nil
		}
		db = opened
	}

	fail := func(err error) (*store.Store, error) {
		if db != nil {
			db.Close()
		}
		return nil, err
	}
	if !promptConfirm(cmd, flags, fmt.Sprintf("Local data is not complete: %s. Run sync now? [Y/n] ", reason)) {
		return fail(fmt.Errorf("local data is not complete: %s; run '7geese-cli sync' first", reason))
	}
	if err := ensureSession(cmd, flags); err != nil {
		return fail(err)
	}
	if db == nil {
		opened, err := store.OpenWithContext(cmd.Context(), dbPath)
		if err != nil {
			return nil, fmt.Errorf("opening local database: %w", err)
		}
		db = opened
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Syncing...")
	reporter := &humanSyncReporter{out: cmd.ErrOrStderr()}
	if err := runSync(cmd.Context(), db, syncOptionsFromFlags(flags, reporter)); err != nil {
		return fail(fmt.Errorf("sync failed: %w", err))
	}
	return db, nil
}

type exportOutput struct {
	ExportedAt    string          `json:"exported_at" jsonschema:"description=ISO 8601 timestamp of when this export was generated"`
	SyncedAt      string          `json:"synced_at" jsonschema:"description=ISO 8601 timestamp of the oldest sync of the datasets in this export"`
	Me            exportProfile   `json:"me" jsonschema:"description=The profile and history of the exported user"`
	DirectReports []exportProfile `json:"direct_reports,omitempty" jsonschema:"description=Direct reports of the session user: profile, objectives and reviews"`
}

type exportProfile struct {
	Profile       exportUser               `json:"profile" jsonschema:"description=User profile information"`
	Objectives    []exportObjective        `json:"objectives" jsonschema:"description=OKRs where the user is an owner, stakeholder, or follower"`
	Reviews       []exportReview           `json:"reviews" jsonschema:"description=Performance reviews in all workflow states"`
	Feedback      *[]exportFeedbackRequest `json:"feedback,omitempty" jsonschema:"description=Feedback requests about the user with the responses. Newest first. Only for the session user"`
	Oneonones     *[]exportMeeting         `json:"oneonones,omitempty" jsonschema:"description=1:1 meeting history with questions and answers from the two participants. Only for the session user"`
	Recognitions  *exportRecognition       `json:"recognitions,omitempty" jsonschema:"description=Recognition badges sent and received. Only for the session user"`
	Checkins      *[]exportUserCheckin     `json:"checkins,omitempty" jsonschema:"description=Check-ins that the user posted, newest first. Only for the session user"`
	Notifications *[]exportNotification    `json:"notifications,omitempty" jsonschema:"description=Notification feed, newest first. Only for the session user"`
}

type exportUser struct {
	ID        int    `json:"id" jsonschema:"description=7Geese user profile ID"`
	Name      string `json:"name" jsonschema:"description=Full name"`
	Email     string `json:"email,omitempty" jsonschema:"description=Work email address"`
	Position  string `json:"position,omitempty" jsonschema:"description=Job title or position"`
	HireDate  string `json:"hire_date,omitempty" jsonschema:"description=Date the user joined the organisation (YYYY-MM-DD)"`
	ManagerID int    `json:"manager_id,omitempty" jsonschema:"description=7Geese user profile ID of the user's direct manager"`
	Manager   string `json:"manager,omitempty" jsonschema:"description=Full name of the user's direct manager"`
}

type exportObjective struct {
	ID              string          `json:"id" jsonschema:"description=Objective ID"`
	Name            string          `json:"name" jsonschema:"description=Objective title"`
	Description     string          `json:"description,omitempty" jsonschema:"description=Optional longer description of the objective"`
	Type            string          `json:"type" jsonschema:"description=Scope of the objective: personal, team, org, or unknown_<n> for a value that 7Geese does not document"`
	ParticipantType string          `json:"participant_type,omitempty" jsonschema:"description=The role of the session user on this objective: owner, stakeholder, or follower. Only for the session user"`
	Progress        float64         `json:"progress" jsonschema:"description=Overall completion percentage (0–100)"`
	StartDate       string          `json:"start_date,omitempty" jsonschema:"description=Objective start date (YYYY-MM-DD)"`
	DueDate         string          `json:"due_date,omitempty" jsonschema:"description=Objective due date (ISO 8601)"`
	CompletedDate   string          `json:"completed_date,omitempty" jsonschema:"description=Date the objective was marked complete (ISO 8601)"`
	Closed          bool            `json:"closed" jsonschema:"description=Whether the objective is closed (completed or abandoned)"`
	KeyResults      []exportKR      `json:"key_results,omitempty" jsonschema:"description=Key results nested under this objective"`
	Checkins        []exportCheckin `json:"checkins,omitempty" jsonschema:"description=Full check-in history of the objective, newest first"`
}

type exportKR struct {
	ID              string  `json:"id" jsonschema:"description=Key result ID"`
	Name            string  `json:"name" jsonschema:"description=Key result title"`
	Description     string  `json:"description,omitempty" jsonschema:"description=Optional longer description"`
	Progress        float64 `json:"progress" jsonschema:"description=Completion percentage (0–100)"`
	MeasurementType string  `json:"measurement_type" jsonschema:"description=How progress is measured: percent, number, currency, boolean, or unknown_<n> for a value that 7Geese does not document"`
	StartingValue   float64 `json:"starting_value" jsonschema:"description=Value at the start of the key result"`
	CurrentValue    float64 `json:"current_value" jsonschema:"description=Current measured value"`
	TargetValue     float64 `json:"target_value" jsonschema:"description=Target value to reach 100% completion"`
}

type exportCheckin struct {
	ID               string   `json:"id" jsonschema:"description=Check-in ID"`
	Date             string   `json:"date" jsonschema:"description=Date the check-in was posted (ISO 8601)"`
	Author           string   `json:"author,omitempty" jsonschema:"description=Full name of the person who posted the check-in"`
	Message          string   `json:"message,omitempty" jsonschema:"description=Free-text update posted with this check-in"`
	ProgressSnapshot *float64 `json:"progress_at_checkin,omitempty" jsonschema:"description=Objective progress percentage recorded at the time of this check-in"`
}

type exportUserCheckin struct {
	ID               string   `json:"id" jsonschema:"description=Check-in ID"`
	Date             string   `json:"date" jsonschema:"description=Date the check-in was posted (ISO 8601)"`
	Message          string   `json:"message,omitempty" jsonschema:"description=Free-text update posted with this check-in"`
	ObjectiveID      string   `json:"objective_id,omitempty" jsonschema:"description=ID of the objective that the check-in is for"`
	ObjectiveName    string   `json:"objective_name,omitempty" jsonschema:"description=Title of the objective at the time of the check-in"`
	ProgressSnapshot *float64 `json:"progress_at_checkin,omitempty" jsonschema:"description=Objective progress percentage recorded at the time of this check-in"`
}

type exportNotification struct {
	ID      string `json:"id" jsonschema:"description=Notification ID"`
	Date    string `json:"date" jsonschema:"description=Date the notification was published (ISO 8601)"`
	Verb    string `json:"verb,omitempty" jsonschema:"description=Type of the event"`
	Actor   string `json:"actor,omitempty" jsonschema:"description=Display name of the person who caused the event"`
	Content string `json:"content,omitempty" jsonschema:"description=Notification text. Can contain HTML"`
}

type exportMeeting struct {
	ID        string         `json:"id" jsonschema:"description=1:1 meeting ID"`
	Date      string         `json:"date" jsonschema:"description=Scheduled date and time of the meeting (ISO 8601)"`
	With      string         `json:"with" jsonschema:"description=Full name of the other participant"`
	Status    string         `json:"status" jsonschema:"description=Meeting status: completed, upcoming (not completed and the start is in the future), incomplete (not completed and the start is in the past), or unknown_<n> for a value that 7Geese does not document"`
	Questions []exportQA     `json:"questions,omitempty" jsonschema:"description=Questions and answers from the two participants. Only for completed meetings"`
	Comments  []exportAnswer `json:"comments,omitempty" jsonschema:"description=Comments on the meeting. Only for completed meetings"`
}

type exportQA struct {
	Question string         `json:"question" jsonschema:"description=The question text"`
	Hint     string         `json:"hint,omitempty" jsonschema:"description=Optional guidance or sub-prompt shown under the question"`
	Answers  []exportAnswer `json:"answers" jsonschema:"description=Answers submitted by each participant"`
}

type exportAnswer struct {
	Author string `json:"author" jsonschema:"description=Full name of the person who wrote this text"`
	Text   string `json:"text" jsonschema:"description=The text"`
}

type exportRecognition struct {
	Received []exportBadge `json:"received" jsonschema:"description=Badges the user received from others"`
	Given    []exportBadge `json:"given" jsonschema:"description=Badges the user sent to others"`
}

type exportReview struct {
	ID                 int                   `json:"id" jsonschema:"description=Performance review ID"`
	Title              string                `json:"title" jsonschema:"description=Name of the review cycle (e.g. 'Q1 2026 Performance Review')"`
	Period             string                `json:"period" jsonschema:"description=Shortest calendar period that contains the start and end dates: a quarter (Q1 2026), a half year (H1 2026), a year (2026), or a range of years (2025-2026)"`
	StartDate          string                `json:"start_date" jsonschema:"description=Review period start date (ISO 8601)"`
	EndDate            string                `json:"end_date" jsonschema:"description=Review period end date (ISO 8601)"`
	State              string                `json:"state" jsonschema:"description=Workflow state: employee_input, manager_input, skip_level_approval, admin_approval, completed, acknowledgment, or unknown_<n> for a value that 7Geese does not document"`
	Position           string                `json:"position,omitempty" jsonschema:"description=The user's job title at the time of the review"`
	Manager            string                `json:"manager,omitempty" jsonschema:"description=Full name of the user's manager for this review"`
	Sections           []exportReviewSection `json:"sections,omitempty" jsonschema:"description=Review form sections containing questions and answers"`
	PeerFeedback       *exportPeerReport     `json:"peer_feedback,omitempty" jsonschema:"description=Curated peer feedback report if one was published for this cycle"`
	FeedbackRequestIDs []int                 `json:"feedback_request_ids,omitempty" jsonschema:"description=IDs of the feedback requests that are attached to this review. For the session user the requests and their responses are in the feedback section"`
}

type exportReviewSection struct {
	Title string           `json:"title" jsonschema:"description=Section heading"`
	Items []exportReviewQA `json:"items" jsonschema:"description=Questions and answers within this section"`
}

type exportReviewQA struct {
	Question string `json:"question" jsonschema:"description=The review question text"`
	Employee string `json:"employee,omitempty" jsonschema:"description=The answer of the reviewed user"`
	Manager  string `json:"manager,omitempty" jsonschema:"description=The answer of the other responder, usually the manager"`
}

type exportPeerReport struct {
	Title     string               `json:"title" jsonschema:"description=Title of the peer feedback request"`
	Published string               `json:"published,omitempty" jsonschema:"description=Date the curated report was published (ISO 8601)"`
	Questions []exportPeerQuestion `json:"questions,omitempty" jsonschema:"description=Questions with anonymised peer answers"`
}

type exportFeedbackRequest struct {
	ID                  int                      `json:"id" jsonschema:"description=Feedback request ID"`
	Title               string                   `json:"title" jsonschema:"description=Title of the feedback request"`
	Visible             bool                     `json:"visible" jsonschema:"description=False if a review refers to this request but 7Geese does not show the request to the session user. Only the ID and the title are then available"`
	Description         string                   `json:"description,omitempty" jsonschema:"description=Message of the feedback request"`
	Status              string                   `json:"status,omitempty" jsonschema:"description=Status that 7Geese gives for the recipient (for example DRAFT or IN_FLIGHT or COMPLETE)"`
	RequestedBy         string                   `json:"requested_by,omitempty" jsonschema:"description=Full name of the person who made the request"`
	Created             string                   `json:"created,omitempty" jsonschema:"description=Time the request was made (ISO 8601)"`
	Sent                string                   `json:"sent,omitempty" jsonschema:"description=Time the request was sent (ISO 8601)"`
	Due                 string                   `json:"due,omitempty" jsonschema:"description=Due time of the responses (ISO 8601)"`
	Asked               []string                 `json:"asked,omitempty" jsonschema:"description=Full names of the people who were asked for feedback. Only the names that 7Geese shows"`
	AskedCount          *int                     `json:"asked_count,omitempty" jsonschema:"description=Number of people who were asked for feedback"`
	ResponsesSubmitted  *int                     `json:"responses_submitted,omitempty" jsonschema:"description=Number of responses that 7Geese reports as submitted"`
	AllResponsesVisible bool                     `json:"all_responses_visible" jsonschema:"description=False if 7Geese does not show all the responses to the session user. The responses list can then be incomplete"`
	Responses           []exportFeedbackResponse `json:"responses" jsonschema:"description=Responses that 7Geese shows to the session user"`
	CuratedReport       *exportPeerReport        `json:"curated_report,omitempty" jsonschema:"description=Curated report of the responses if one is available"`
}

type exportFeedbackResponse struct {
	From      string                  `json:"from,omitempty" jsonschema:"description=Full name of the person who gave the feedback. Absent if 7Geese hides the name"`
	Submitted string                  `json:"submitted,omitempty" jsonschema:"description=Time the response was submitted (ISO 8601). Absent if the response is not submitted"`
	Answers   []exportFeedbackAnswer  `json:"answers" jsonschema:"description=Answers of this response"`
	Comments  []exportFeedbackComment `json:"comments,omitempty" jsonschema:"description=Comments on the response"`
}

type exportFeedbackAnswer struct {
	Question string                  `json:"question" jsonschema:"description=The question text"`
	Hint     string                  `json:"hint,omitempty" jsonschema:"description=Guidance shown under the question"`
	Answer   string                  `json:"answer" jsonschema:"description=The answer: text or a number or the selected options"`
	Scale    string                  `json:"scale,omitempty" jsonschema:"description=Range of a number answer if the question defines one"`
	Comments []exportFeedbackComment `json:"comments,omitempty" jsonschema:"description=Comments on the answer"`
}

type exportFeedbackComment struct {
	Author  string `json:"author,omitempty" jsonschema:"description=Full name of the person who wrote the comment. Absent if 7Geese hides the name"`
	Created string `json:"created" jsonschema:"description=Time the comment was written (ISO 8601)"`
	Text    string `json:"text" jsonschema:"description=The comment text"`
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

type exporter struct {
	db          *store.Store
	sessionUser int
	since       *time.Time
	sinceDate   string
	now         time.Time
	oldestSync  time.Time
}

func buildExport(db *store.Store, userFilter, since string, now time.Time) (*exportOutput, error) {
	sessionUser, found, err := storedSessionUser(db)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("the local database has no synced data; run '7geese-cli sync'")
	}
	e := &exporter{db: db, sessionUser: sessionUser, now: now}
	if since != "" {
		start, err := time.ParseInLocation(exportDateLayout, since, now.Location())
		if err != nil {
			return nil, fmt.Errorf("invalid --since value %q: use the format YYYY-MM-DD", since)
		}
		e.since = &start
		e.sinceDate = since
	}
	if err := e.use(sessionDataset(resourceUserProfile)); err != nil {
		return nil, err
	}

	target := sessionUser
	if userFilter != "" {
		target, err = e.resolveUser(userFilter)
		if err != nil {
			return nil, err
		}
	}
	me, err := e.profile(target)
	if err != nil {
		return nil, err
	}
	out := &exportOutput{Me: *me}
	if target == sessionUser {
		reportIDs, err := e.queryIDs(
			`SELECT id FROM userprofile_view WHERE manager_id = ? AND id <> ? ORDER BY id`, sessionUser, sessionUser)
		if err != nil {
			return nil, fmt.Errorf("listing direct reports: %w", err)
		}
		for _, reportID := range reportIDs {
			report, err := e.profile(reportID)
			if err != nil {
				return nil, fmt.Errorf("direct report %d: %w", reportID, err)
			}
			out.DirectReports = append(out.DirectReports, *report)
		}
	}
	out.ExportedAt = now.UTC().Format(time.RFC3339)
	out.SyncedAt = e.oldestSync.UTC().Format(time.RFC3339)
	return out, nil
}

func (e *exporter) use(key store.DatasetKey) error {
	syncedAt, synced, err := e.db.DatasetSyncedAt(key)
	if err != nil {
		return err
	}
	if !synced {
		return fmt.Errorf("the dataset %s is not synced; run '7geese-cli sync' (sync gets your data and the objectives and reviews of your direct reports)", key)
	}
	if e.oldestSync.IsZero() || syncedAt.Before(e.oldestSync) {
		e.oldestSync = syncedAt
	}
	return nil
}

func (e *exporter) atOrAfterSince(what, timestamp string) (bool, error) {
	if e.since == nil {
		return true, nil
	}
	instant, err := parseTimestamp(what, timestamp)
	if err != nil {
		return false, err
	}
	return !instant.Before(*e.since), nil
}

func (e *exporter) objectiveInRange(node gqlObjectiveNode) (bool, error) {
	if e.since == nil || !node.Closed {
		return true, nil
	}
	dates := []struct {
		what  string
		value *string
	}{
		{"due date", node.DueDatetime},
		{"completion date", node.CompletedDatetime},
	}
	dated := false
	for _, date := range dates {
		if date.value == nil || *date.value == "" {
			continue
		}
		dated = true
		keep, err := e.atOrAfterSince(fmt.Sprintf("objective %d %s", node.PK, date.what), *date.value)
		if err != nil {
			return false, err
		}
		if keep {
			return true, nil
		}
	}
	if !dated {
		return false, fmt.Errorf("closed objective %d has no due date and no completion date; --since cannot tell if it is in the period", node.PK)
	}
	return false, nil
}

func (e *exporter) queryIDs(query string, args ...any) ([]int, error) {
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id sql.NullInt64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if !id.Valid || id.Int64 <= 0 {
			return nil, errors.New("a row has no valid id")
		}
		ids = append(ids, int(id.Int64))
	}
	return ids, rows.Err()
}

func (e *exporter) queryData(query string, args ...any) ([]string, error) {
	rows, err := e.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []string
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		all = append(all, data)
	}
	return all, rows.Err()
}

func (e *exporter) resolveUser(filter string) (int, error) {
	if id, err := strconv.Atoi(filter); err == nil {
		ids, err := e.queryIDs(`SELECT id FROM userprofile_view WHERE id = ?`, id)
		if err != nil {
			return 0, fmt.Errorf("finding user %d: %w", id, err)
		}
		if len(ids) == 0 {
			return 0, fmt.Errorf("user %d is not in the synced user profiles", id)
		}
		return id, nil
	}
	ids, err := e.queryIDs(
		`SELECT id FROM userprofile_view WHERE lower(email) = lower(?) OR lower(full_name) = lower(?) ORDER BY id`,
		filter, filter)
	if err != nil {
		return 0, fmt.Errorf("finding user %q: %w", filter, err)
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("user %q is not in the synced user profiles; use the full name, the email or the numeric ID", filter)
	}
	if len(ids) > 1 {
		return 0, fmt.Errorf("%q matches %d users (IDs %v); use the numeric ID", filter, len(ids), ids)
	}
	return ids[0], nil
}

func (e *exporter) profile(profileID int) (*exportProfile, error) {
	user, err := e.user(profileID)
	if err != nil {
		return nil, err
	}
	objectives, err := e.objectives(profileID)
	if err != nil {
		return nil, err
	}
	reviews, err := e.reviews(profileID)
	if err != nil {
		return nil, err
	}
	profile := &exportProfile{Profile: *user, Objectives: objectives, Reviews: reviews}
	if profileID != e.sessionUser {
		return profile, nil
	}

	feedback, err := e.feedback(reviews)
	if err != nil {
		return nil, err
	}
	profile.Feedback = &feedback

	meetings, err := e.meetings()
	if err != nil {
		return nil, err
	}
	recognitions, err := e.recognitions()
	if err != nil {
		return nil, err
	}
	checkins, err := e.checkins()
	if err != nil {
		return nil, err
	}
	notifications, err := e.notifications()
	if err != nil {
		return nil, err
	}
	profile.Oneonones = &meetings
	profile.Recognitions = recognitions
	profile.Checkins = &checkins
	profile.Notifications = &notifications
	return profile, nil
}

func (e *exporter) user(profileID int) (*exportUser, error) {
	var name, email, position, hireDate sql.NullString
	var managerID sql.NullInt64
	err := e.db.DB().QueryRow(
		`SELECT full_name, email, position, hire_date, manager_id FROM userprofile_view WHERE id = ?`, profileID,
	).Scan(&name, &email, &position, &hireDate, &managerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user %d is not in the synced user profiles; run '7geese-cli sync'", profileID)
	}
	if err != nil {
		return nil, fmt.Errorf("reading user profile %d: %w", profileID, err)
	}
	if name.String == "" {
		return nil, fmt.Errorf("user profile %d has no name", profileID)
	}
	user := &exportUser{
		ID:       profileID,
		Name:     name.String,
		Email:    email.String,
		Position: position.String,
		HireDate: hireDate.String,
	}
	if !managerID.Valid {
		return user, nil
	}
	user.ManagerID = int(managerID.Int64)
	var managerName sql.NullString
	err = e.db.DB().QueryRow(`SELECT full_name FROM userprofile_view WHERE id = ?`, user.ManagerID).Scan(&managerName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading manager profile %d: %w", user.ManagerID, err)
	}
	user.Manager = managerName.String
	return user, nil
}

func (e *exporter) objectives(ownerID int) ([]exportObjective, error) {
	if err := e.use(userDataset(resourceUserObjectives, ownerID)); err != nil {
		return nil, err
	}
	rows, err := e.queryData(
		`SELECT data FROM objective_view WHERE owner_id = ? ORDER BY closed ASC, due_datetime DESC, id ASC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("reading objectives of user %d: %w", ownerID, err)
	}
	objectives := make([]exportObjective, 0, len(rows))
	for _, raw := range rows {
		var node gqlObjectiveNode
		if err := json.Unmarshal([]byte(raw), &node); err != nil {
			return nil, fmt.Errorf("decoding a stored objective of user %d: %w", ownerID, err)
		}
		keep, err := e.objectiveInRange(node)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		objective, err := exportObjectiveFromNode(node, ownerID == e.sessionUser)
		if err != nil {
			return nil, err
		}
		objectives = append(objectives, *objective)
	}
	return objectives, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func exportObjectiveFromNode(node gqlObjectiveNode, includeRole bool) (*exportObjective, error) {
	if len(node.Checkins.Edges) != node.Checkins.TotalCount {
		return nil, fmt.Errorf("objective %d has %d of %d check-ins in the local database; run '7geese-cli sync'", node.PK, len(node.Checkins.Edges), node.Checkins.TotalCount)
	}
	objective := &exportObjective{
		ID:            strconv.Itoa(node.PK),
		Name:          node.Name,
		Description:   node.Description,
		Type:          objectiveTypeName(node.ObjectiveType),
		Progress:      node.Progress,
		StartDate:     stringValue(node.StartDate),
		DueDate:       stringValue(node.DueDatetime),
		CompletedDate: stringValue(node.CompletedDatetime),
		Closed:        node.Closed,
	}
	if includeRole {
		objective.ParticipantType = participantTypeName(node.CurrentUserParticipantType)
	}
	for _, edge := range node.KeyResults.Edges {
		kr := edge.Node
		objective.KeyResults = append(objective.KeyResults, exportKR{
			ID:              strconv.Itoa(kr.PK),
			Name:            kr.Name,
			Description:     kr.Description,
			Progress:        kr.Progress,
			MeasurementType: krMeasurementTypeName(kr.MeasurementType),
			StartingValue:   kr.StartingValue,
			CurrentValue:    kr.CurrentValue,
			TargetValue:     kr.TargetValue,
		})
	}
	checkins := make([]gqlObjectiveCheckin, 0, len(node.Checkins.Edges))
	for _, edge := range node.Checkins.Edges {
		checkins = append(checkins, edge.Node)
	}
	sort.SliceStable(checkins, func(i, j int) bool {
		if checkins[i].Created != checkins[j].Created {
			return checkins[i].Created > checkins[j].Created
		}
		return checkins[i].PK > checkins[j].PK
	})
	for _, checkin := range checkins {
		progress, err := checkinProgress(checkin.Metadata)
		if err != nil {
			return nil, fmt.Errorf("check-in %d of objective %d: %w", checkin.PK, node.PK, err)
		}
		objective.Checkins = append(objective.Checkins, exportCheckin{
			ID:               strconv.Itoa(checkin.PK),
			Date:             checkin.Created,
			Author:           checkin.User.FullName,
			Message:          checkin.Message,
			ProgressSnapshot: progress,
		})
	}
	return objective, nil
}

func checkinProgress(metadata json.RawMessage) (*float64, error) {
	if len(metadata) == 0 || string(metadata) == "null" {
		return nil, nil
	}
	var decoded struct {
		Metadata *struct {
			Objective *struct {
				Snapshot *struct {
					Progress *float64 `json:"progress"`
				} `json:"snapshot"`
			} `json:"objective"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		return nil, fmt.Errorf("decoding metadata: %w", err)
	}
	if decoded.Metadata == nil || decoded.Metadata.Objective == nil || decoded.Metadata.Objective.Snapshot == nil {
		return nil, nil
	}
	return decoded.Metadata.Objective.Snapshot.Progress, nil
}

func participantTypeName(t int) string {
	switch t {
	case 0:
		return ""
	case 1:
		return "owner"
	case 2:
		return "stakeholder"
	case 3:
		return "follower"
	default:
		return fmt.Sprintf("unknown_%d", t)
	}
}

func krMeasurementTypeName(t int) string {
	switch t {
	case 1:
		return "percent"
	case 3:
		return "number"
	case 6:
		return "currency"
	case 7:
		return "boolean"
	default:
		return fmt.Sprintf("unknown_%d", t)
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
		return fmt.Sprintf("unknown_%d", t)
	}
}

func meetingStatusName(status int64, start string, now time.Time) (string, error) {
	switch status {
	case oneononeStatusCompleted:
		return "completed", nil
	case 0:
		startTime, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return "", fmt.Errorf("start time %q is not an ISO 8601 timestamp", start)
		}
		if startTime.After(now) {
			return "upcoming", nil
		}
		return "incomplete", nil
	default:
		return fmt.Sprintf("unknown_%d", status), nil
	}
}

func (e *exporter) meetings() ([]exportMeeting, error) {
	if err := e.use(sessionDataset(resourceOneonones)); err != nil {
		return nil, err
	}
	if err := e.use(sessionDataset(resourceFinalizedOneonones)); err != nil {
		return nil, err
	}
	rows, err := e.db.Query(
		`SELECT id, start_datetime, status, creator_id, creator_name, target_id, target_name
		FROM oneonone_view WHERE (creator_id = ? OR target_id = ?)
		ORDER BY start_datetime DESC, id DESC`, e.sessionUser, e.sessionUser)
	if err != nil {
		return nil, fmt.Errorf("reading 1:1 meetings: %w", err)
	}
	defer rows.Close()

	type meetingRow struct {
		id          int
		start       string
		status      int64
		completed   bool
		with        string
		statusLabel string
	}
	var meetingRows []meetingRow
	for rows.Next() {
		var id, status, creatorID, targetID sql.NullInt64
		var start, creatorName, targetName sql.NullString
		if err := rows.Scan(&id, &start, &status, &creatorID, &creatorName, &targetID, &targetName); err != nil {
			return nil, fmt.Errorf("reading a 1:1 meeting: %w", err)
		}
		if !id.Valid || !start.Valid || !status.Valid {
			return nil, fmt.Errorf("1:1 meeting %d has no id, no start time or no status", id.Int64)
		}
		keep, err := e.atOrAfterSince(fmt.Sprintf("1:1 meeting %d start time", id.Int64), start.String)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		with := targetName.String
		if targetID.Valid && targetID.Int64 == int64(e.sessionUser) {
			with = creatorName.String
		}
		if with == "" {
			return nil, fmt.Errorf("1:1 meeting %d has no name for the other participant", id.Int64)
		}
		label, err := meetingStatusName(status.Int64, start.String, e.now)
		if err != nil {
			return nil, fmt.Errorf("1:1 meeting %d: %w", id.Int64, err)
		}
		meetingRows = append(meetingRows, meetingRow{
			id:          int(id.Int64),
			start:       start.String,
			completed:   status.Int64 == oneononeStatusCompleted,
			with:        with,
			statusLabel: label,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading 1:1 meetings: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("reading 1:1 meetings: %w", err)
	}

	meetings := make([]exportMeeting, 0, len(meetingRows))
	for _, row := range meetingRows {
		meeting := exportMeeting{ID: strconv.Itoa(row.id), Date: row.start, With: row.with, Status: row.statusLabel}
		if row.completed {
			raw, found, err := e.db.GetRecord(sessionDataset(resourceFinalizedOneonones), meeting.ID)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, fmt.Errorf("the completed 1:1 meeting %d has no notes in the local database; run '7geese-cli sync'", row.id)
			}
			var finalized gqlMeeting
			if err := json.Unmarshal(raw, &finalized); err != nil {
				return nil, fmt.Errorf("decoding the stored notes of 1:1 meeting %d: %w", row.id, err)
			}
			meeting.Questions, meeting.Comments = exportMeetingNotes(finalized)
		}
		meetings = append(meetings, meeting)
	}
	return meetings, nil
}

func exportMeetingNotes(meeting gqlMeeting) ([]exportQA, []exportAnswer) {
	var questions []exportQA
	for _, edge := range meeting.QuestionSet.Questions.Edges {
		question := edge.Node
		qa := exportQA{Question: question.Content, Hint: question.Description, Answers: []exportAnswer{}}
		for _, noteEdge := range question.Notes.Edges {
			text := strings.TrimSpace(noteEdge.Node.Content)
			if text == "" {
				continue
			}
			qa.Answers = append(qa.Answers, exportAnswer{Author: noteEdge.Node.Creator.FullName, Text: text})
		}
		if qa.Question == "" && len(qa.Answers) == 0 {
			continue
		}
		questions = append(questions, qa)
	}
	var comments []exportAnswer
	for _, edge := range meeting.Comments.Edges {
		text := strings.TrimSpace(edge.Node.Content)
		if text == "" {
			continue
		}
		comments = append(comments, exportAnswer{Author: edge.Node.Creator.FullName, Text: text})
	}
	return questions, comments
}

func (e *exporter) badges(view, dateColumn string) ([]exportBadge, error) {
	rows, err := e.db.Query(fmt.Sprintf(
		`SELECT id, %s, sender_name, recipient_name, badge, message FROM %s ORDER BY %s DESC, id DESC`,
		dateColumn, view, dateColumn))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", view, err)
	}
	defer rows.Close()
	badges := []exportBadge{}
	for rows.Next() {
		var id, date, from, to, badge, message sql.NullString
		if err := rows.Scan(&id, &date, &from, &to, &badge, &message); err != nil {
			return nil, fmt.Errorf("reading %s: %w", view, err)
		}
		if id.String == "" || date.String == "" {
			return nil, fmt.Errorf("a row of %s has no id or no date (id %q)", view, id.String)
		}
		keep, err := e.atOrAfterSince(fmt.Sprintf("%s row %s date", view, id.String), date.String)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		badges = append(badges, exportBadge{
			ID:      id.String,
			Date:    date.String,
			From:    from.String,
			To:      to.String,
			Badge:   badge.String,
			Message: message.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", view, err)
	}
	return badges, nil
}

func (e *exporter) recognitions() (*exportRecognition, error) {
	if err := e.use(sessionDataset(resourceRecognitionGiven)); err != nil {
		return nil, err
	}
	if err := e.use(sessionDataset(resourceRecognitionGot)); err != nil {
		return nil, err
	}
	given, err := e.badges("recognition_given_view", "created")
	if err != nil {
		return nil, err
	}
	received, err := e.badges("recognition_received_view", "published")
	if err != nil {
		return nil, err
	}
	return &exportRecognition{Received: received, Given: given}, nil
}

func (e *exporter) checkins() ([]exportUserCheckin, error) {
	if err := e.use(sessionDataset(resourceCheckins)); err != nil {
		return nil, err
	}
	rows, err := e.db.Query(
		`SELECT id, created, message, objective_id, objective_name, progress FROM checkin_view ORDER BY created DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading check-ins: %w", err)
	}
	defer rows.Close()
	checkins := []exportUserCheckin{}
	for rows.Next() {
		var id, objectiveID sql.NullInt64
		var created, message, objectiveName sql.NullString
		var progress sql.NullFloat64
		if err := rows.Scan(&id, &created, &message, &objectiveID, &objectiveName, &progress); err != nil {
			return nil, fmt.Errorf("reading a check-in: %w", err)
		}
		if !id.Valid || created.String == "" {
			return nil, fmt.Errorf("check-in %d has no id or no date", id.Int64)
		}
		keep, err := e.atOrAfterSince(fmt.Sprintf("check-in %d date", id.Int64), created.String)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		checkin := exportUserCheckin{
			ID:            strconv.FormatInt(id.Int64, 10),
			Date:          created.String,
			Message:       message.String,
			ObjectiveName: objectiveName.String,
		}
		if objectiveID.Valid {
			checkin.ObjectiveID = strconv.FormatInt(objectiveID.Int64, 10)
		}
		if progress.Valid {
			value := progress.Float64
			checkin.ProgressSnapshot = &value
		}
		checkins = append(checkins, checkin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading check-ins: %w", err)
	}
	return checkins, nil
}

func (e *exporter) notifications() ([]exportNotification, error) {
	if err := e.use(sessionDataset(resourceNotifications)); err != nil {
		return nil, err
	}
	rows, err := e.db.Query(
		`SELECT id, published, verb, actor_name, content FROM notification_view ORDER BY published DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading notifications: %w", err)
	}
	defer rows.Close()
	notifications := []exportNotification{}
	for rows.Next() {
		var id, published, verb, actor, content sql.NullString
		if err := rows.Scan(&id, &published, &verb, &actor, &content); err != nil {
			return nil, fmt.Errorf("reading a notification: %w", err)
		}
		if id.String == "" || published.String == "" {
			return nil, fmt.Errorf("notification %q has no id or no date", id.String)
		}
		keep, err := e.atOrAfterSince(fmt.Sprintf("notification %s date", id.String), published.String)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		notifications = append(notifications, exportNotification{
			ID:      id.String,
			Date:    published.String,
			Verb:    verb.String,
			Actor:   actor.String,
			Content: content.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading notifications: %w", err)
	}
	return notifications, nil
}

func (e *exporter) reviews(ownerID int) ([]exportReview, error) {
	if err := e.use(userDataset(resourceUserSnapshots, ownerID)); err != nil {
		return nil, err
	}
	rows, err := e.queryData(
		`SELECT data FROM review_view WHERE owner_id = ? ORDER BY start_date DESC, id DESC`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("reading reviews of user %d: %w", ownerID, err)
	}
	reviews := make([]exportReview, 0, len(rows))
	for _, raw := range rows {
		var snapshot gqlSnapshotFull
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, fmt.Errorf("decoding a stored review of user %d: %w", ownerID, err)
		}
		if e.since != nil {
			if _, err := time.Parse(exportDateLayout, snapshot.EndDate); err != nil {
				return nil, fmt.Errorf("review %d end date %q is not a YYYY-MM-DD date", snapshot.PK, snapshot.EndDate)
			}
			if snapshot.EndDate < e.sinceDate {
				continue
			}
		}
		review, err := exportReviewFromSnapshot(snapshot, ownerID)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, *review)
	}
	return reviews, nil
}

var knownReviewAnswerTypes = map[string]bool{
	"TextItemAnswerNode":        true,
	"MultiOptionItemAnswerNode": true,
	"RangeItemAnswerNode":       true,
}

func reviewAnswerText(answer gqlSnapshotAnswer) string {
	switch {
	case answer.Answer != "":
		return stripHTMLTags(answer.Answer)
	case answer.RangeAnswer != nil:
		return strconv.FormatFloat(*answer.RangeAnswer, 'f', -1, 64)
	case len(answer.Choices.Edges) > 0:
		options := make([]string, 0, len(answer.Choices.Edges))
		for _, edge := range answer.Choices.Edges {
			options = append(options, edge.Node.Option.Title)
		}
		return strings.Join(options, ", ")
	default:
		return ""
	}
}

func appendAnswer(existing, text string) string {
	if existing == "" {
		return text
	}
	return existing + "\n\n" + text
}

func exportReviewFromSnapshot(snapshot gqlSnapshotFull, ownerID int) (*exportReview, error) {
	period, err := reviewPeriod(snapshot.StartDate, snapshot.EndDate)
	if err != nil {
		return nil, fmt.Errorf("review %d: %w", snapshot.PK, err)
	}
	review := &exportReview{
		ID:        snapshot.PK,
		Title:     snapshot.SnapshotGroup.Title,
		Period:    period,
		StartDate: snapshot.StartDate,
		EndDate:   snapshot.EndDate,
		State:     snapshotStateName(snapshot.WorkflowState),
		Position:  snapshot.Position,
	}
	if snapshot.Manager != nil {
		review.Manager = snapshot.Manager.FullName
	}

	sectionItems := map[int]bool{}
	for _, sectionEdge := range snapshot.SnapshotGroup.Sections.Edges {
		for _, itemEdge := range sectionEdge.Node.Items.Edges {
			sectionItems[itemEdge.Node.PK] = true
		}
	}

	type answerPair struct{ employee, manager string }
	answers := map[int]*answerPair{}
	for _, edge := range snapshot.Answers.Edges {
		answer := edge.Node
		text := reviewAnswerText(answer)
		if text == "" {
			if !knownReviewAnswerTypes[answer.TypeName] {
				return nil, fmt.Errorf("review %d has an answer of the type %q that the export cannot read", snapshot.PK, answer.TypeName)
			}
			continue
		}
		if answer.Item.PK == 0 || !sectionItems[answer.Item.PK] {
			return nil, fmt.Errorf("review %d has an answer for item %d that is not in a section of the review", snapshot.PK, answer.Item.PK)
		}
		if answer.Responder.PK == 0 {
			return nil, fmt.Errorf("review %d has an answer for item %d with no responder", snapshot.PK, answer.Item.PK)
		}
		pair := answers[answer.Item.PK]
		if pair == nil {
			pair = &answerPair{}
			answers[answer.Item.PK] = pair
		}
		if answer.Responder.PK == ownerID {
			pair.employee = appendAnswer(pair.employee, text)
		} else {
			pair.manager = appendAnswer(pair.manager, text)
		}
	}

	for _, sectionEdge := range snapshot.SnapshotGroup.Sections.Edges {
		section := sectionEdge.Node
		var items []exportReviewQA
		for _, itemEdge := range section.Items.Edges {
			pair := answers[itemEdge.Node.PK]
			if pair == nil {
				continue
			}
			items = append(items, exportReviewQA{
				Question: itemEdge.Node.Title,
				Employee: pair.employee,
				Manager:  pair.manager,
			})
		}
		if len(items) > 0 {
			review.Sections = append(review.Sections, exportReviewSection{Title: section.Title, Items: items})
		}
	}

	if snapshot.PeerFeedbackRequest != nil && snapshot.PeerFeedbackRequest.CuratedReport != nil {
		peer, err := exportCuratedReport(snapshot.PeerFeedbackRequest.Title, snapshot.PeerFeedbackRequest.CuratedReport)
		if err != nil {
			return nil, fmt.Errorf("review %d: %w", snapshot.PK, err)
		}
		review.PeerFeedback = peer
	}
	review.FeedbackRequestIDs, err = snapshot.feedbackRequestIDs()
	if err != nil {
		return nil, err
	}
	return review, nil
}

func exportCuratedReport(title string, report *gqlCuratedReport) (*exportPeerReport, error) {
	peer := &exportPeerReport{Title: title, Published: report.PublishedDatetime}
	for _, questionEdge := range report.SortedCuratedQuestions.Edges {
		var question struct {
			Title string `json:"title"`
		}
		if len(questionEdge.Node.Question) > 0 {
			if err := json.Unmarshal(questionEdge.Node.Question, &question); err != nil {
				return nil, fmt.Errorf("decoding a peer feedback question: %w", err)
			}
		}
		peerQuestion := exportPeerQuestion{Question: question.Title}
		for _, answerEdge := range questionEdge.Node.Answers.Edges {
			if text := stripHTMLTags(answerEdge.Node.Answer); text != "" {
				peerQuestion.Answers = append(peerQuestion.Answers, text)
			}
		}
		if peerQuestion.Question == "" && len(peerQuestion.Answers) == 0 {
			continue
		}
		peer.Questions = append(peer.Questions, peerQuestion)
	}
	if len(peer.Questions) == 0 {
		return nil, nil
	}
	return peer, nil
}

func (e *exporter) feedback(reviews []exportReview) ([]exportFeedbackRequest, error) {
	if err := e.use(sessionDataset(resourceFeedbackRequests)); err != nil {
		return nil, err
	}
	rows, err := e.queryData(`SELECT data FROM feedback_request_view ORDER BY created DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("reading feedback requests: %w", err)
	}
	inReview := map[int]int{}
	for _, review := range reviews {
		for _, id := range review.FeedbackRequestIDs {
			inReview[id] = review.ID
		}
	}
	synced := map[int]bool{}
	requests := make([]exportFeedbackRequest, 0, len(rows))
	for _, raw := range rows {
		var request gqlFeedbackRequest
		if err := json.Unmarshal([]byte(raw), &request); err != nil {
			return nil, fmt.Errorf("decoding a stored feedback request: %w", err)
		}
		synced[request.PK] = true
		_, linked := inReview[request.PK]
		keep, err := e.feedbackRequestInRange(request, linked)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		exported, err := exportFeedbackRequestFromNode(request)
		if err != nil {
			return nil, err
		}
		requests = append(requests, *exported)
	}
	linkedIDs := make([]int, 0, len(inReview))
	for id := range inReview {
		linkedIDs = append(linkedIDs, id)
	}
	sort.Ints(linkedIDs)
	for _, id := range linkedIDs {
		if !synced[id] {
			return nil, fmt.Errorf("review %d refers to feedback request %d, which is not in the synced feedback requests; run '7geese-cli sync'", inReview[id], id)
		}
	}
	return requests, nil
}

func (e *exporter) feedbackRequestInRange(request gqlFeedbackRequest, inExportedReview bool) (bool, error) {
	if e.since == nil || inExportedReview {
		return true, nil
	}
	if request.NotVisible {
		return false, nil
	}
	dates := []string{request.Created}
	if request.SentDate != nil && *request.SentDate != "" {
		dates = append(dates, *request.SentDate)
	}
	for _, edge := range request.Feedbacks.Edges {
		if submitted := edge.Node.SubmittedTimestamp; submitted != nil && *submitted != "" {
			dates = append(dates, *submitted)
		}
	}
	for _, date := range dates {
		keep, err := e.atOrAfterSince(fmt.Sprintf("feedback request %d date", request.PK), date)
		if err != nil {
			return false, err
		}
		if keep {
			return true, nil
		}
	}
	return false, nil
}

func exportFeedbackComments(comments gqlFeedbackComments) []exportFeedbackComment {
	var exported []exportFeedbackComment
	for _, edge := range comments.Edges {
		comment := exportFeedbackComment{Created: edge.Node.Created, Text: stripHTMLTags(edge.Node.Comment)}
		if edge.Node.Creator != nil {
			comment.Author = edge.Node.Creator.FullName
		}
		exported = append(exported, comment)
	}
	return exported
}

func feedbackAnswerText(requestID int, answer gqlFeedbackAnswer) (string, error) {
	switch answer.TypeName {
	case "TextAnswerNode":
		if answer.Text == nil {
			return "", fmt.Errorf("feedback request %d has a text answer %d with no text", requestID, answer.PK)
		}
		return stripHTMLTags(*answer.Text), nil
	case "RangeAnswerNode", "NpsAnswerNode", "LikertAnswerNode":
		if answer.Number == nil {
			return "", fmt.Errorf("feedback request %d has a number answer %d with no number", requestID, answer.PK)
		}
		return strconv.Itoa(*answer.Number), nil
	case "MultiOptionAnswerNode":
		if answer.Choices == nil {
			return "", fmt.Errorf("feedback request %d has a choice answer %d with no choices", requestID, answer.PK)
		}
		options := make([]string, 0, len(answer.Choices.Edges))
		for _, edge := range answer.Choices.Edges {
			options = append(options, edge.Node.Option.Title)
		}
		return strings.Join(options, ", "), nil
	default:
		return "", fmt.Errorf("feedback request %d has an answer of the type %q that the export cannot read", requestID, answer.TypeName)
	}
}

func feedbackAnswerScale(question gqlFeedbackQuestion) string {
	if question.Start == nil || question.End == nil {
		return ""
	}
	end := func(value int, text string) string {
		if text == "" {
			return strconv.Itoa(value)
		}
		return fmt.Sprintf("%d (%s)", value, text)
	}
	return end(*question.Start, question.StartText) + " to " + end(*question.End, question.EndText)
}

func exportFeedbackRequestFromNode(request gqlFeedbackRequest) (*exportFeedbackRequest, error) {
	if request.NotVisible {
		return &exportFeedbackRequest{ID: request.PK, Title: request.Title, Responses: []exportFeedbackResponse{}}, nil
	}
	if request.NumResponsesSubmitted == nil || request.CanViewAllResponses == nil {
		return nil, fmt.Errorf("stored feedback request %d does not say how many responses it has or if they are visible; run '7geese-cli sync'", request.PK)
	}
	askedCount := len(request.Responders.Edges)
	exported := &exportFeedbackRequest{
		ID:                  request.PK,
		Title:               request.Title,
		Visible:             true,
		Description:         stripHTMLTags(request.Description),
		Status:              request.StatusByRecipient,
		Created:             request.Created,
		Sent:                stringValue(request.SentDate),
		Due:                 stringValue(request.DueDatetime),
		AskedCount:          &askedCount,
		ResponsesSubmitted:  request.NumResponsesSubmitted,
		AllResponsesVisible: *request.CanViewAllResponses,
		Responses:           []exportFeedbackResponse{},
	}
	if request.Creator != nil {
		exported.RequestedBy = request.Creator.FullName
	}
	for _, edge := range request.Responders.Edges {
		if edge.Node.Profile != nil && edge.Node.Profile.FullName != "" {
			exported.Asked = append(exported.Asked, edge.Node.Profile.FullName)
		}
	}
	for _, feedbackEdge := range request.Feedbacks.Edges {
		feedback := feedbackEdge.Node
		response := exportFeedbackResponse{
			Submitted: stringValue(feedback.SubmittedTimestamp),
			Answers:   []exportFeedbackAnswer{},
			Comments:  exportFeedbackComments(feedback.Comments),
		}
		if feedback.Responder != nil && feedback.Responder.Profile != nil {
			response.From = feedback.Responder.Profile.FullName
		}
		for _, answerEdge := range feedback.Answers.Edges {
			answer := answerEdge.Node
			if answer.Question == nil {
				return nil, fmt.Errorf("feedback request %d has an answer %d with no question", request.PK, answer.PK)
			}
			text, err := feedbackAnswerText(request.PK, answer)
			if err != nil {
				return nil, err
			}
			if text == "" {
				continue
			}
			exportedAnswer := exportFeedbackAnswer{
				Question: answer.Question.Title,
				Hint:     stripHTMLTags(answer.Question.Description),
				Answer:   text,
				Comments: exportFeedbackComments(answer.Comments),
			}
			if answer.Number != nil {
				exportedAnswer.Scale = feedbackAnswerScale(*answer.Question)
			}
			response.Answers = append(response.Answers, exportedAnswer)
		}
		exported.Responses = append(exported.Responses, response)
	}
	if request.CuratedReport != nil {
		report, err := exportCuratedReport(request.Title, request.CuratedReport)
		if err != nil {
			return nil, fmt.Errorf("feedback request %d: %w", request.PK, err)
		}
		exported.CuratedReport = report
	}
	return exported, nil
}

func reviewPeriod(startDate, endDate string) (string, error) {
	parse := func(name, value string) (time.Time, error) {
		if len(value) < len(exportDateLayout) {
			return time.Time{}, fmt.Errorf("%s %q is not a date", name, value)
		}
		date, err := time.Parse(exportDateLayout, value[:len(exportDateLayout)])
		if err != nil {
			return time.Time{}, fmt.Errorf("%s %q is not a date", name, value)
		}
		return date, nil
	}
	start, err := parse("start date", startDate)
	if err != nil {
		return "", err
	}
	end, err := parse("end date", endDate)
	if err != nil {
		return "", err
	}
	if end.Before(start) {
		return "", fmt.Errorf("end date %q is before start date %q", endDate, startDate)
	}
	if start.Year() != end.Year() {
		return fmt.Sprintf("%d-%d", start.Year(), end.Year()), nil
	}
	startQuarter := (int(start.Month()) + 2) / 3
	endQuarter := (int(end.Month()) + 2) / 3
	if startQuarter == endQuarter {
		return fmt.Sprintf("Q%d %d", startQuarter, start.Year()), nil
	}
	startHalf := (startQuarter + 1) / 2
	endHalf := (endQuarter + 1) / 2
	if startHalf == endHalf {
		return fmt.Sprintf("H%d %d", startHalf, start.Year()), nil
	}
	return strconv.Itoa(start.Year()), nil
}
