// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

func syncedFixture(t *testing.T) *syncFixture {
	t.Helper()
	x := newSyncFixture(t)
	x.mustSync(t)
	return x
}

func mustExport(t *testing.T, x *syncFixture, user, since string) *exportOutput {
	t.Helper()
	out, err := buildExport(x.db, user, since, fakeNow)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return out
}

func findObjective(t *testing.T, objectives []exportObjective, id string) exportObjective {
	t.Helper()
	for _, objective := range objectives {
		if objective.ID == id {
			return objective
		}
	}
	t.Fatalf("objective %s is not in the export", id)
	return exportObjective{}
}

func TestExport_SessionUser(t *testing.T) {
	x := syncedFixture(t)
	out := mustExport(t, x, "", "")

	if out.ExportedAt != "2026-06-17T12:00:00Z" {
		t.Errorf("exported_at = %q", out.ExportedAt)
	}
	syncedAt, err := time.Parse(time.RFC3339, out.SyncedAt)
	if err != nil || time.Since(syncedAt) > time.Minute || time.Since(syncedAt) < 0 {
		t.Errorf("synced_at = %q, want the time of the sync", out.SyncedAt)
	}

	profile := out.Me.Profile
	want := exportUser{ID: fakeMe, Name: "Alex Morgan", Email: "alex@example.com", Position: "Engineer", HireDate: "2020-01-02", ManagerID: fakeManager, Manager: "Blake Chen"}
	if profile != want {
		t.Errorf("profile = %+v, want %+v", profile, want)
	}

	if out.Me.Oneonones == nil {
		t.Fatal("the export of the session user has no oneonones")
	}
	meetings := *out.Me.Oneonones
	if len(meetings) != 4 {
		t.Fatalf("oneonones = %d, want 4", len(meetings))
	}
	wantMeetings := []struct{ id, with, status string }{
		{"3", "Blake Chen", "upcoming"},
		{"4", "Blake Chen", "incomplete"},
		{"2", "Casey Diaz", "completed"},
		{"1", "Blake Chen", "completed"},
	}
	for i, wantMeeting := range wantMeetings {
		got := meetings[i]
		if got.ID != wantMeeting.id || got.With != wantMeeting.with || got.Status != wantMeeting.status {
			t.Errorf("meeting %d = id %s with %q status %s, want %+v", i, got.ID, got.With, got.Status, wantMeeting)
		}
	}
	if len(meetings[0].Questions) != 0 || len(meetings[1].Questions) != 0 {
		t.Errorf("meetings that are not completed have questions")
	}
	first := meetings[3]
	if len(first.Questions) != 2 {
		t.Fatalf("meeting 1 has %d questions, want 2 (the empty question is omitted)", len(first.Questions))
	}
	if answers := first.Questions[0].Answers; len(answers) != 2 || answers[0] != (exportAnswer{Author: "Alex Morgan", Text: "Shipped X"}) || answers[1] != (exportAnswer{Author: "Blake Chen", Text: "Great"}) {
		t.Errorf("answers of question 1 = %+v", answers)
	}
	if answers := first.Questions[1].Answers; len(answers) != 12 || answers[11].Text != "note 12" {
		t.Errorf("question 2 has %d answers, want all 12", len(answers))
	}
	if len(first.Comments) != 1 || first.Comments[0] != (exportAnswer{Author: "Blake Chen", Text: "Good meeting"}) {
		t.Errorf("comments = %+v", first.Comments)
	}

	if len(out.Me.Objectives) != 59 {
		t.Fatalf("objectives = %d, want 59", len(out.Me.Objectives))
	}
	if order := out.Me.Objectives[0].ID + "," + out.Me.Objectives[1].ID + "," + out.Me.Objectives[2].ID + "," + out.Me.Objectives[3].ID; order != "11,14,12,13" {
		t.Errorf("objective order = %s, want 11,14,12,13", order)
	}
	shipped := findObjective(t, out.Me.Objectives, "11")
	if shipped.ParticipantType != "owner" || shipped.Type != "personal" || shipped.Closed || shipped.DueDate != "2026-12-31T23:59:59+00:00" || shipped.CompletedDate != "" {
		t.Errorf("objective 11 = %+v", shipped)
	}
	if len(shipped.KeyResults) != 1 || shipped.KeyResults[0].MeasurementType != "number" || shipped.KeyResults[0].ID != "111" {
		t.Errorf("key results of objective 11 = %+v", shipped.KeyResults)
	}
	if len(shipped.Checkins) != 250 {
		t.Fatalf("objective 11 has %d check-ins, want 250", len(shipped.Checkins))
	}
	newest, oldest := shipped.Checkins[0], shipped.Checkins[249]
	if newest.ID != "5250" || newest.Date != "2026-06-16T09:00:00+00:00" || newest.Author != "Alex Morgan" || newest.Message != "update 250" || newest.ProgressSnapshot == nil || *newest.ProgressSnapshot != 25 {
		t.Errorf("newest check-in = %+v", newest)
	}
	if oldest.ID != "5001" || oldest.Message != "update 1" {
		t.Errorf("oldest check-in = %+v", oldest)
	}
	if got := findObjective(t, out.Me.Objectives, "12"); got.ParticipantType != "stakeholder" || got.Type != "team" || got.Checkins[0].Author != "Blake Chen" {
		t.Errorf("objective 12 = %+v", got)
	}
	if got := findObjective(t, out.Me.Objectives, "14"); got.ParticipantType != "follower" || len(got.Checkins) != 0 {
		t.Errorf("objective 14 = %+v", got)
	}
	if got := findObjective(t, out.Me.Objectives, "2000"); got.Type != "org" || !got.Closed {
		t.Errorf("objective 2000 = %+v", got)
	}

	if len(out.Me.Reviews) != 2 {
		t.Fatalf("reviews = %d, want 2", len(out.Me.Reviews))
	}
	half, quarter := out.Me.Reviews[0], out.Me.Reviews[1]
	if half.ID != 72 || half.Period != "H1 2026" || half.State != "employee_input" || len(half.Sections) != 0 {
		t.Errorf("review 72 = %+v", half)
	}
	if quarter.ID != 71 || quarter.Period != "Q1 2026" || quarter.State != "completed" || quarter.Title != "Q1 review" || quarter.Manager != "Blake Chen" {
		t.Errorf("review 71 = %+v", quarter)
	}
	if len(quarter.Sections) != 1 || len(quarter.Sections[0].Items) != 1 {
		t.Fatalf("sections of review 71 = %+v", quarter.Sections)
	}
	if item := quarter.Sections[0].Items[0]; item != (exportReviewQA{Question: "Question 711", Employee: "My answer", Manager: "Manager answer"}) {
		t.Errorf("review answer = %+v", item)
	}

	if out.Me.Recognitions == nil || len(out.Me.Recognitions.Received) != 120 || len(out.Me.Recognitions.Given) != 2 {
		t.Fatalf("recognitions = %+v", out.Me.Recognitions)
	}
	if given := out.Me.Recognitions.Given[0]; given != (exportBadge{ID: "7002", Date: "2026-05-01T09:00:00+00:00", From: "Alex Morgan", To: "Casey Diaz", Badge: "Team", Message: "well done"}) {
		t.Errorf("newest given badge = %+v", given)
	}
	if received := out.Me.Recognitions.Received[0]; received != (exportBadge{ID: "r120", Date: "2026-06-16T08:00:00Z", From: "Blake Chen", To: "Alex Morgan", Badge: "Star", Message: "r message 120"}) {
		t.Errorf("newest received badge = %+v", received)
	}

	if out.Me.Checkins == nil || len(*out.Me.Checkins) != 3 {
		t.Fatalf("checkins = %+v", out.Me.Checkins)
	}
	if checkin := (*out.Me.Checkins)[0]; checkin.ID != "6001" || checkin.Message != "this week" || checkin.ObjectiveID != "11" || checkin.ObjectiveName != "Objective 11" || checkin.ProgressSnapshot == nil || *checkin.ProgressSnapshot != 25 {
		t.Errorf("newest check-in = %+v", checkin)
	}
	if out.Me.Notifications == nil || len(*out.Me.Notifications) != 120 {
		t.Fatalf("notifications = %+v", out.Me.Notifications)
	}
	if notification := (*out.Me.Notifications)[0]; notification != (exportNotification{ID: "n120", Date: "2026-06-16T08:00:00Z", Verb: "recognize", Actor: "Blake Chen", Content: "n content 120"}) {
		t.Errorf("newest notification = %+v", notification)
	}

	if len(out.DirectReports) != 1 {
		t.Fatalf("direct_reports = %d, want 1", len(out.DirectReports))
	}
	report := out.DirectReports[0]
	if report.Profile.ID != fakeReport || report.Profile.Name != "Casey Diaz" || report.Profile.Manager != "Alex Morgan" {
		t.Errorf("direct report profile = %+v", report.Profile)
	}
	if len(report.Objectives) != 1 || report.Objectives[0].ID != "31" || report.Objectives[0].ParticipantType != "" {
		t.Errorf("direct report objectives = %+v", report.Objectives)
	}
	if len(report.Reviews) != 1 || report.Reviews[0].Period != "2025" {
		t.Fatalf("direct report reviews = %+v", report.Reviews)
	}
	if item := report.Reviews[0].Sections[0].Items[0]; item.Employee != "Report answer" || item.Manager != "Manager view" {
		t.Errorf("direct report review answer = %+v", item)
	}
	if report.Oneonones != nil || report.Recognitions != nil || report.Checkins != nil || report.Notifications != nil {
		t.Errorf("the direct report has sections of the session user")
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encoding the direct report: %v", err)
	}
	for _, key := range []string{"oneonones", "recognitions", "checkins\":", "notifications"} {
		if strings.Contains(string(encoded), "\""+key) {
			t.Errorf("the JSON of the direct report has the key %s", key)
		}
	}
}

func TestExport_OtherUser(t *testing.T) {
	x := syncedFixture(t)
	for _, filter := range []string{"300", "casey diaz", "CASEY@example.com"} {
		out := mustExport(t, x, filter, "")
		if out.Me.Profile.ID != fakeReport {
			t.Errorf("--user %q exported user %d, want %d", filter, out.Me.Profile.ID, fakeReport)
		}
		if out.Me.Oneonones != nil || out.Me.Recognitions != nil || len(out.DirectReports) != 0 {
			t.Errorf("--user %q has data of the session user", filter)
		}
		if len(out.Me.Objectives) != 1 || len(out.Me.Reviews) != 1 {
			t.Errorf("--user %q: %d objectives, %d reviews, want 1 and 1", filter, len(out.Me.Objectives), len(out.Me.Reviews))
		}
	}

	cases := []struct{ filter, wantError string }{
		{"Nobody Here", "is not in the synced user profiles"},
		{"999", "user 999 is not in the synced user profiles"},
		{"Sam Twin", "matches 2 users (IDs [500 501])"},
		{"400", "user_objectives[400] is not synced"},
		{"Casey", "is not in the synced user profiles"},
	}
	for _, c := range cases {
		_, err := buildExport(x.db, c.filter, "", fakeNow)
		if err == nil || !strings.Contains(err.Error(), c.wantError) {
			t.Errorf("--user %q: error = %v, want %q", c.filter, err, c.wantError)
		}
	}
}

func TestExport_Since(t *testing.T) {
	x := syncedFixture(t)
	out := mustExport(t, x, "", "2026-04-01")

	meetingIDs := []string{}
	for _, meeting := range *out.Me.Oneonones {
		meetingIDs = append(meetingIDs, meeting.ID)
	}
	if strings.Join(meetingIDs, ",") != "3,4,2" {
		t.Errorf("oneonones = %v, want 3,4,2", meetingIDs)
	}
	objectiveIDs := []string{}
	for _, objective := range out.Me.Objectives {
		objectiveIDs = append(objectiveIDs, objective.ID)
	}
	if strings.Join(objectiveIDs, ",") != "11,14,12" {
		t.Errorf("objectives = %v, want 11,14,12", objectiveIDs)
	}
	if len(out.Me.Reviews) != 1 || out.Me.Reviews[0].ID != 72 {
		t.Errorf("reviews = %+v, want only 72", out.Me.Reviews)
	}
	if len(*out.Me.Checkins) != 2 {
		t.Errorf("checkins = %d, want 2", len(*out.Me.Checkins))
	}
	if len(*out.Me.Notifications) != 120 {
		t.Errorf("notifications = %d, want 120", len(*out.Me.Notifications))
	}
	if len(out.Me.Recognitions.Given) != 1 || out.Me.Recognitions.Given[0].ID != "7002" || len(out.Me.Recognitions.Received) != 120 {
		t.Errorf("recognitions: %d given, %d received", len(out.Me.Recognitions.Given), len(out.Me.Recognitions.Received))
	}
	report := out.DirectReports[0]
	if report.Reviews == nil || len(report.Reviews) != 0 {
		t.Errorf("direct report reviews = %+v, want an empty list", report.Reviews)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	if !strings.Contains(string(encoded), `"reviews":[]`) {
		t.Errorf("an empty review list is not encoded as []: %s", encoded)
	}
}

func TestExport_SinceStartsAtMidnightInTheLocalZone(t *testing.T) {
	x := syncedFixture(t)
	cases := []struct {
		zone *time.Location
		want int
	}{
		{time.UTC, 57},
		{time.FixedZone("east", 2*3600), 59},
		{time.FixedZone("west", -5*3600), 52},
	}
	for _, c := range cases {
		out, err := buildExport(x.db, "", "2026-06-14", fakeNow.In(c.zone))
		if err != nil {
			t.Fatalf("%s: export: %v", c.zone, err)
		}
		if got := len(out.Me.Recognitions.Received); got != c.want {
			t.Errorf("%s: received recognitions = %d, want %d", c.zone, got, c.want)
		}
		if got := len(*out.Me.Notifications); got != c.want {
			t.Errorf("%s: notifications = %d, want %d", c.zone, got, c.want)
		}
		if got := len(*out.Me.Checkins); got != 1 {
			t.Errorf("%s: checkins = %d, want 1", c.zone, got)
		}
		if got := len(out.Me.Recognitions.Given); got != 0 {
			t.Errorf("%s: given recognitions = %d, want 0", c.zone, got)
		}
	}
}

func TestExport_SinceFailsOnRecordsItCannotDate(t *testing.T) {
	t.Run("closed objective with no dates", func(t *testing.T) {
		x := syncedFixture(t)
		data := json.RawMessage(`{"pk":11,"name":"x","closed":true,"checkins":{"totalCount":0,"edges":[]}}`)
		if err := x.db.ReplaceAll(userDataset(resourceUserObjectives, fakeMe), []store.Record{{ID: "11", Data: data}}); err != nil {
			t.Fatalf("replacing objectives: %v", err)
		}
		_, err := buildExport(x.db, "", "2026-04-01", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "closed objective 11 has no due date and no completion date") {
			t.Errorf("error = %v, want an objective that has no dates", err)
		}
	})
	t.Run("check-in with no date", func(t *testing.T) {
		x := syncedFixture(t)
		data := json.RawMessage(`{"id":1,"message":"x"}`)
		if err := x.db.ReplaceAll(sessionDataset(resourceCheckins), []store.Record{{ID: "1", Data: data}}); err != nil {
			t.Fatalf("replacing check-ins: %v", err)
		}
		_, err := buildExport(x.db, "", "2026-04-01", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "check-in 1 has no id or no date") {
			t.Errorf("error = %v, want a check-in that has no date", err)
		}
	})
	t.Run("meeting start that is not a timestamp", func(t *testing.T) {
		x := syncedFixture(t)
		data := json.RawMessage(`{"id":2,"status":1,"start_datetime":"next week","creator":{"id":100,"user":{"first_name":"Alex","last_name":"Morgan"}},"target":{"id":300,"user":{"first_name":"Casey","last_name":"Diaz"}}}`)
		if err := x.db.ReplaceAll(sessionDataset(resourceOneonones), []store.Record{{ID: "2", Data: data}}); err != nil {
			t.Fatalf("replacing meetings: %v", err)
		}
		_, err := buildExport(x.db, "", "2026-04-01", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "1:1 meeting 2 start time \"next week\" is not an ISO 8601 timestamp") {
			t.Errorf("error = %v, want a start time that is not a timestamp", err)
		}
	})
	t.Run("since value that is not a date", func(t *testing.T) {
		x := syncedFixture(t)
		_, err := buildExport(x.db, "", "2026-4-1", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "invalid --since value") {
			t.Errorf("error = %v, want an invalid --since value", err)
		}
	})
}

func TestExportAndWeek_RefuseRowsOfADatasetThatIsNotSynced(t *testing.T) {
	x := syncedFixture(t)
	if _, err := x.db.DB().Exec(`DELETE FROM sync_state WHERE resource_type = ?`, resourceOneonones); err != nil {
		t.Fatalf("removing sync state: %v", err)
	}
	if ids := x.ids(t, sessionDataset(resourceOneonones)); len(ids) == 0 {
		t.Fatalf("the 1:1 rows are gone, want rows with no sync state")
	}
	want := "the dataset oneonones is not synced"
	if _, err := buildExport(x.db, "", "", fakeNow); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("export error = %v, want %q", err, want)
	}
	if _, err := buildWeekSummary(x.db, fakeNow); err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("week error = %v, want %q", err, want)
	}
	reason, err := missingSyncReason(x.db)
	if err != nil || reason == "" {
		t.Errorf("missingSyncReason = %q (err %v), want a reason to sync", reason, err)
	}
}

func TestExport_FailsOnIncompleteLocalData(t *testing.T) {
	t.Run("no sync", func(t *testing.T) {
		x := newSyncFixture(t)
		_, err := buildExport(x.db, "", "", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "run '7geese-cli sync'") {
			t.Errorf("error = %v, want an instruction to run sync", err)
		}
	})
	t.Run("completed meeting without notes", func(t *testing.T) {
		x := syncedFixture(t)
		if err := x.db.ReplaceAll(sessionDataset(resourceFinalizedOneonones), nil); err != nil {
			t.Fatalf("clearing notes: %v", err)
		}
		_, err := buildExport(x.db, "", "", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "completed 1:1 meeting 2 has no notes") {
			t.Errorf("error = %v, want a missing notes error", err)
		}
	})
	t.Run("part of the check-in history", func(t *testing.T) {
		x := syncedFixture(t)
		data := json.RawMessage(`{"pk":11,"name":"x","checkins":{"totalCount":5,"edges":[{"node":{"pk":1,"created":"2026-01-01T00:00:00+00:00"}}]}}`)
		if err := x.db.ReplaceAll(userDataset(resourceUserObjectives, fakeMe), []store.Record{{ID: "11", Data: data}}); err != nil {
			t.Fatalf("replacing objectives: %v", err)
		}
		_, err := buildExport(x.db, "", "", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "objective 11 has 1 of 5 check-ins") {
			t.Errorf("error = %v, want a check-in count error", err)
		}
	})
	t.Run("dataset that is not synced", func(t *testing.T) {
		x := syncedFixture(t)
		if _, err := x.db.PurgeExcept([]store.DatasetKey{
			sessionDataset(resourceSessionUser), sessionDataset(resourceUserProfile),
			userDataset(resourceUserObjectives, fakeMe), userDataset(resourceUserSnapshots, fakeMe),
		}); err != nil {
			t.Fatalf("purging: %v", err)
		}
		_, err := buildExport(x.db, "", "", fakeNow)
		if err == nil || !strings.Contains(err.Error(), "the dataset oneonones is not synced") {
			t.Errorf("error = %v, want a dataset error", err)
		}
	})
}

func TestReviewPeriod(t *testing.T) {
	cases := []struct{ start, end, want string }{
		{"2026-01-01", "2026-03-31", "Q1 2026"},
		{"2026-07-01", "2026-09-30", "Q3 2026"},
		{"2026-10-05", "2026-12-31", "Q4 2026"},
		{"2026-01-01", "2026-06-30", "H1 2026"},
		{"2026-07-01", "2026-12-31", "H2 2026"},
		{"2026-01-01", "2026-12-31", "2026"},
		{"2026-04-01", "2026-09-30", "2026"},
		{"2025-10-01", "2026-03-31", "2025-2026"},
		{"2026-02-10T00:00:00+00:00", "2026-02-20", "Q1 2026"},
	}
	for _, c := range cases {
		got, err := reviewPeriod(c.start, c.end)
		if err != nil || got != c.want {
			t.Errorf("reviewPeriod(%q, %q) = %q, %v, want %q", c.start, c.end, got, err, c.want)
		}
	}
	for _, c := range [][2]string{{"", "2026-01-01"}, {"2026-01-01", "soon"}, {"2026-05-01", "2026-01-01"}, {"2026-13-01", "2026-12-01"}} {
		if got, err := reviewPeriod(c[0], c[1]); err == nil {
			t.Errorf("reviewPeriod(%q, %q) = %q, want an error", c[0], c[1], got)
		}
	}
}

func TestMeetingStatusName(t *testing.T) {
	cases := []struct {
		status int64
		start  string
		want   string
	}{
		{1, "2026-06-20T10:00:00+00:00", "completed"},
		{1, "2020-01-01T10:00:00+00:00", "completed"},
		{0, "2026-06-20T10:00:00+00:00", "upcoming"},
		{0, "2026-06-17T13:30:00+02:00", "incomplete"},
		{0, "2026-06-17T08:30:00-05:00", "upcoming"},
		{0, "2026-05-01T10:00:00+00:00", "incomplete"},
		{7, "2026-05-01T10:00:00+00:00", "unknown_7"},
	}
	for _, c := range cases {
		got, err := meetingStatusName(c.status, c.start, fakeNow)
		if err != nil || got != c.want {
			t.Errorf("meetingStatusName(%d, %q) = %q, %v, want %q", c.status, c.start, got, err, c.want)
		}
	}
	if got, err := meetingStatusName(0, "tomorrow", fakeNow); err == nil {
		t.Errorf("meetingStatusName with a bad start time = %q, want an error", got)
	}
}

func TestEnumNames(t *testing.T) {
	measurement := map[int]string{1: "percent", 3: "number", 6: "currency", 7: "boolean", 0: "unknown_0", 2: "unknown_2"}
	for value, want := range measurement {
		if got := krMeasurementTypeName(value); got != want {
			t.Errorf("krMeasurementTypeName(%d) = %q, want %q", value, got, want)
		}
	}
	objectiveTypes := map[int]string{1: "personal", 2: "team", 3: "org", 4: "unknown_4"}
	for value, want := range objectiveTypes {
		if got := objectiveTypeName(value); got != want {
			t.Errorf("objectiveTypeName(%d) = %q, want %q", value, got, want)
		}
	}
	roles := map[int]string{0: "", 1: "owner", 2: "stakeholder", 3: "follower", 9: "unknown_9"}
	for value, want := range roles {
		if got := participantTypeName(value); got != want {
			t.Errorf("participantTypeName(%d) = %q, want %q", value, got, want)
		}
	}
	if got := snapshotStateName(9); got != "unknown_9" {
		t.Errorf("snapshotStateName(9) = %q, want unknown_9", got)
	}
}

func snapshotFromFake(t *testing.T, review fakeReview) gqlSnapshotFull {
	t.Helper()
	data, err := json.Marshal(review.Data)
	if err != nil {
		t.Fatalf("encoding snapshot: %v", err)
	}
	var snapshot gqlSnapshotFull
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatalf("decoding snapshot: %v", err)
	}
	return snapshot
}

func TestExportReviewFromSnapshot_Answers(t *testing.T) {
	text := "TextItemAnswerNode"
	joined := snapshotFromFake(t, fakeSnapshot(1, 4, "t", "2026-01-01", "2026-03-31", []int{10}, []fakeAnswer{
		{ItemPK: 10, ResponderPK: 7, TypeName: text, Text: "first"},
		{ItemPK: 10, ResponderPK: 8, TypeName: text, Text: "second"},
		{ItemPK: 10, ResponderPK: 5, TypeName: text, Text: "own"},
	}))
	review, err := exportReviewFromSnapshot(joined, 5)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if item := review.Sections[0].Items[0]; item.Employee != "own" || item.Manager != "first\n\nsecond" {
		t.Errorf("answers = %+v, want the two other answers joined", item)
	}

	failures := []struct {
		name      string
		answer    fakeAnswer
		wantError string
	}{
		{"item not in a section", fakeAnswer{ItemPK: 99, ResponderPK: 5, TypeName: text, Text: "x"}, "item 99 that is not in a section"},
		{"no item", fakeAnswer{ItemPK: 0, ResponderPK: 5, TypeName: text, Text: "x"}, "item 0 that is not in a section"},
		{"no responder", fakeAnswer{ItemPK: 10, ResponderPK: 0, TypeName: text, Text: "x"}, "with no responder"},
		{"unknown type with no content", fakeAnswer{ItemPK: 10, ResponderPK: 5, TypeName: "MatrixItemAnswerNode", Text: ""}, `"MatrixItemAnswerNode"`},
	}
	for _, c := range failures {
		snapshot := snapshotFromFake(t, fakeSnapshot(1, 4, "t", "2026-01-01", "2026-03-31", []int{10}, []fakeAnswer{c.answer}))
		if _, err := exportReviewFromSnapshot(snapshot, 5); err == nil || !strings.Contains(err.Error(), c.wantError) {
			t.Errorf("%s: error = %v, want %q", c.name, err, c.wantError)
		}
	}

	badDates := snapshotFromFake(t, fakeSnapshot(1, 4, "t", "", "2026-03-31", []int{10}, nil))
	if _, err := exportReviewFromSnapshot(badDates, 5); err == nil || !strings.Contains(err.Error(), "is not a date") {
		t.Errorf("bad dates: error = %v, want a date error", err)
	}
}

func runExportCommand(flags *rootFlags, args ...string) (string, string, error) {
	cmd := newMeExportCmd(flags)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestMeExportCommand_NeedsConfirmationToSync(t *testing.T) {
	x := newSyncFixture(t)
	t.Setenv("API_7GEESE_CONFIG", x.configPath)
	missing := filepath.Join(t.TempDir(), "sub", "data.db")

	_, _, err := runExportCommand(&rootFlags{timeout: 5 * time.Second}, "--db", missing)
	if err == nil || !strings.Contains(err.Error(), "the local database does not exist; run '7geese-cli sync' first") {
		t.Fatalf("error = %v, want an instruction to run sync", err)
	}
	if _, statErr := os.Stat(missing); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("the database file was created although the sync was not confirmed (stat error %v)", statErr)
	}
	if count := x.fake.requestCount("/status/"); count != 0 {
		t.Errorf("%d requests were sent without a confirmation", count)
	}

	_, _, err = runExportCommand(&rootFlags{timeout: 5 * time.Second}, "--db", x.dbPath)
	if err == nil || !strings.Contains(err.Error(), "no sync was completed for this database") {
		t.Errorf("error = %v, want the reason that no sync was completed", err)
	}
}

func TestMeExportCommand_YesRunsSyncFirst(t *testing.T) {
	x := newSyncFixture(t)
	t.Setenv("API_7GEESE_CONFIG", x.configPath)
	dbPath := filepath.Join(t.TempDir(), "sub", "data.db")

	stdout, stderr, err := runExportCommand(&rootFlags{timeout: 5 * time.Second, yes: true}, "--db", dbPath)
	if err != nil {
		t.Fatalf("export: %v\n%s", err, stderr)
	}
	var out exportOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not the export JSON: %v\n%s", err, stdout)
	}
	if out.Me.Profile.ID != fakeMe || len(out.Me.Objectives) != 59 || len(out.DirectReports) != 1 {
		t.Errorf("export = profile %d, %d objectives, %d direct reports", out.Me.Profile.ID, len(out.Me.Objectives), len(out.DirectReports))
	}
	if !strings.Contains(stderr, "Sync complete: 323 records in 15 datasets") {
		t.Errorf("stderr has no sync result: %s", stderr)
	}

	before := x.fake.requestCount("/status/")
	if _, _, err := runExportCommand(&rootFlags{timeout: 5 * time.Second}, "--db", dbPath); err != nil {
		t.Fatalf("second export: %v", err)
	}
	if after := x.fake.requestCount("/status/"); after != before {
		t.Errorf("the second export connected to 7Geese although the data is synced")
	}
}

func TestMeExportCommand_LogsInThroughTheBrowser(t *testing.T) {
	x := newSyncFixture(t)
	t.Setenv("API_7GEESE_CONFIG", x.configPath)
	writeFakeConfig(t, x.configPath, x.fake.server.URL, "")
	flags := &rootFlags{timeout: 5 * time.Second, yes: true}

	_, _, err := runExportCommand(flags, "--db", x.dbPath)
	if err == nil || !strings.Contains(err.Error(), "auth login failed") {
		t.Fatalf("error = %v, want a login failure", err)
	}

	browserLogin = func() (string, string, error) { return fakeSession, fakeCSRF, nil }
	stdout, stderr, err := runExportCommand(flags, "--db", x.dbPath)
	if err != nil {
		t.Fatalf("export: %v\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"name": "Alex Morgan"`) {
		t.Errorf("stdout has no profile: %.300s", stdout)
	}
	cfg, err := config.Load(x.configPath)
	if err != nil || cfg.SevengeeseSession != fakeSession {
		t.Errorf("the session was not saved to the config (err %v)", err)
	}
}

func TestMeExportCommand_FlagsAndOutputFile(t *testing.T) {
	x := syncedFixture(t)
	flags := &rootFlags{timeout: 5 * time.Second}

	if _, _, err := runExportCommand(flags, "--db", x.dbPath, "--since", "last week"); err == nil || !strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error = %v, want a date format error", err)
	}

	target := filepath.Join(t.TempDir(), "export.json")
	stdout, _, err := runExportCommand(flags, "--db", x.dbPath, "--output", target, "--user", "300")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !strings.Contains(stdout, "Exported to "+target) {
		t.Errorf("stdout = %q", stdout)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("reading the output file: %v", err)
	}
	var out exportOutput
	if err := json.Unmarshal(data, &out); err != nil || out.Me.Profile.ID != fakeReport {
		t.Errorf("output file: profile %d, err %v", out.Me.Profile.ID, err)
	}
}
