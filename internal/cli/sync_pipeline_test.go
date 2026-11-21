// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

func TestRunSync_StoresAllDatasets(t *testing.T) {
	x := newSyncFixture(t)
	x.mustSync(t)

	want := map[string]int{
		"session_user":         1,
		"userprofile":          6,
		"oneonones":            4,
		"finalized_oneonones":  2,
		"oneononenotes":        2,
		"feedbackrequest":      0,
		"peer_feedback":        0,
		"performancecycles":    1,
		"recognitionbadges":    2,
		"recognition_received": 120,
		"checkins":             3,
		"notifications":        120,
		"user_objectives[100]": 59,
		"user_snapshots[100]":  2,
		"user_objectives[300]": 1,
		"user_snapshots[300]":  1,
	}
	keys, err := x.db.Datasets()
	if err != nil {
		t.Fatalf("listing datasets: %v", err)
	}
	got := map[string]int{}
	for _, key := range keys {
		got[key.String()] = len(x.ids(t, key))
	}
	if len(got) != len(want) {
		t.Errorf("datasets = %v, want %v", got, want)
	}
	for name, count := range want {
		if stored, ok := got[name]; !ok || stored != count {
			t.Errorf("dataset %s has %d records (present %v), want %d", name, stored, ok, count)
		}
	}
	for name, count := range want {
		if name == "session_user" {
			continue
		}
		if reported, ok := x.reporter.done[name]; !ok || reported != count {
			t.Errorf("reporter count of %s = %d (present %v), want %d", name, reported, ok, count)
		}
	}
	if len(x.reporter.failed) != 0 || len(x.reporter.purged) != 0 {
		t.Errorf("failed = %v, purged = %v, want none", x.reporter.failed, x.reporter.purged)
	}
	if x.fake.recognitionWraps != 0 {
		t.Errorf("the recognition feed was restarted %d times through the offset=None link", x.fake.recognitionWraps)
	}

	raw, found, err := x.db.GetRecord(userDataset(resourceUserObjectives, fakeMe), "11")
	if err != nil || !found {
		t.Fatalf("objective 11: found %v, err %v", found, err)
	}
	var objective gqlObjectiveNode
	if err := json.Unmarshal(raw, &objective); err != nil {
		t.Fatalf("decoding objective 11: %v", err)
	}
	if len(objective.Checkins.Edges) != 250 || objective.Checkins.TotalCount != 250 {
		t.Errorf("objective 11 has %d check-ins (totalCount %d), want 250", len(objective.Checkins.Edges), objective.Checkins.TotalCount)
	}
	if objective.Checkins.PageInfo != nil || objective.Owners.PageInfo != nil || objective.KeyResults.PageInfo != nil {
		t.Errorf("page information was stored with objective 11")
	}

	cfg, err := config.Load(x.configPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.SevengeeseUserID != 0 {
		t.Errorf("sync wrote user_id %d to the config, want no change to the config", cfg.SevengeeseUserID)
	}
	review, found, err := x.db.GetRecord(userDataset(resourceUserSnapshots, fakeMe), "71")
	if err != nil || !found {
		t.Fatalf("review 71: found %v, err %v", found, err)
	}
	if strings.Contains(string(review), "pageInfo") {
		t.Errorf("page information was stored with review 71: %s", review)
	}
	owner, found, err := storedSessionUser(x.db)
	if err != nil || !found || owner != fakeMe {
		t.Errorf("stored session user = %d (found %v, err %v), want %d", owner, found, err, fakeMe)
	}
}

func TestRunSync_RemovesDeletedRecordsAndRefreshesChanges(t *testing.T) {
	x := newSyncFixture(t)
	x.mustSync(t)

	x.fake.change(func() {
		meetings := x.fake.lists["/api/v1/oneonones/"]
		x.fake.lists["/api/v1/oneonones/"] = append(meetings[:1:1], meetings[2:]...)
		delete(x.fake.meetings, 2)
		x.fake.lists["/api/v1/checkins/"] = x.fake.lists["/api/v1/checkins/"][:2]
		x.fake.objectives[fakeMe]["ownerOrStakeholder"][1].Name = "Grow the team faster"
		x.fake.objectives[fakeMe]["follower"] = x.fake.objectives[fakeMe]["follower"][:1]
		x.fake.notificationFeed = x.fake.notificationFeed[:10]
		x.fake.reviews[fakeMe] = x.fake.reviews[fakeMe][:1]
	})
	x.mustSync(t)

	if ids := x.ids(t, sessionDataset(resourceOneonones)); strings.Join(ids, ",") != "1,3,4" {
		t.Errorf("oneonones = %v, want 1,3,4", ids)
	}
	if ids := x.ids(t, sessionDataset(resourceFinalizedOneonones)); strings.Join(ids, ",") != "1" {
		t.Errorf("finalized_oneonones = %v, want 1", ids)
	}
	if ids := x.ids(t, sessionDataset(resourceCheckins)); strings.Join(ids, ",") != "6001,6002" {
		t.Errorf("checkins = %v, want 6001,6002", ids)
	}
	if ids := x.ids(t, userDataset(resourceUserObjectives, fakeMe)); strings.Join(ids, ",") != "11,12,13,14" {
		t.Errorf("objectives = %v, want 11,12,13,14", ids)
	}
	if ids := x.ids(t, sessionDataset(resourceNotifications)); len(ids) != 10 {
		t.Errorf("notifications = %d, want 10", len(ids))
	}
	if ids := x.ids(t, userDataset(resourceUserSnapshots, fakeMe)); strings.Join(ids, ",") != "71" {
		t.Errorf("reviews = %v, want 71", ids)
	}
	raw, found, err := x.db.GetRecord(userDataset(resourceUserObjectives, fakeMe), "12")
	if err != nil || !found {
		t.Fatalf("objective 12: found %v, err %v", found, err)
	}
	if !strings.Contains(string(raw), "Grow the team faster") {
		t.Errorf("objective 12 was not refreshed: %s", raw)
	}
}

func TestRunSync_FailedDatasetKeepsEarlierDataAndDoesNotPurge(t *testing.T) {
	x := newSyncFixture(t)
	x.mustSync(t)

	x.fake.change(func() {
		x.fake.failures["/api/v1/oneononenotes/"] = -1
		x.fake.lists["/api/v1/oneononenotes/"] = nil
		x.fake.lists["/api/v1/checkins/"] = x.fake.lists["/api/v1/checkins/"][:2]
		x.fake.lists["/api/v1/userprofile/"][2] = fakeProfile(fakeReport, "Casey", "Diaz", fakeManager)
	})
	x.reporter = newRecordingReporter()
	err := x.sync()
	if err == nil {
		t.Fatal("sync returned no error although oneononenotes failed")
	}
	if !strings.Contains(err.Error(), "oneononenotes") || !strings.Contains(err.Error(), "1 of 13") {
		t.Errorf("error = %q, want it to name oneononenotes and 1 of 13", err)
	}
	if _, ok := x.reporter.failed["oneononenotes"]; !ok || len(x.reporter.failed) != 1 {
		t.Errorf("failed = %v, want only oneononenotes", x.reporter.failed)
	}
	if ids := x.ids(t, sessionDataset(resourceOneononeNotes)); strings.Join(ids, ",") != "9001,9002" {
		t.Errorf("oneononenotes = %v, want the earlier 9001,9002", ids)
	}
	if ids := x.ids(t, sessionDataset(resourceCheckins)); len(ids) != 2 {
		t.Errorf("checkins = %v, want 2 records from the new sync", ids)
	}
	if names := x.datasetNames(t); !containsString(names, "user_objectives[300]") {
		t.Errorf("user_objectives[300] was purged although the sync failed: %v", names)
	}
	if len(x.reporter.purged) != 0 {
		t.Errorf("purged = %v, want none", x.reporter.purged)
	}

	x.fake.change(func() {
		delete(x.fake.failures, "/api/v1/oneononenotes/")
	})
	x.reporter = newRecordingReporter()
	x.mustSync(t)
	names := x.datasetNames(t)
	if containsString(names, "user_objectives[300]") || containsString(names, "user_snapshots[300]") {
		t.Errorf("datasets of the former direct report stay after a full sync: %v", names)
	}
	if !containsString(x.reporter.purged, "user_objectives[300]") || !containsString(x.reporter.purged, "user_snapshots[300]") {
		t.Errorf("purged = %v, want the two datasets of user 300", x.reporter.purged)
	}
	if ids := x.ids(t, sessionDataset(resourceOneononeNotes)); len(ids) != 0 {
		t.Errorf("oneononenotes = %v, want none", ids)
	}
}

func TestRunSync_StatusFailureSyncsNothing(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() { x.fake.failures["/status/"] = -1 })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "reading the current user from 7Geese") {
		t.Fatalf("error = %v, want a current user error", err)
	}
	if names := x.datasetNames(t); len(names) != 0 {
		t.Errorf("datasets = %v, want none", names)
	}
	if count := x.fake.dataRequestCount(); count != 0 {
		t.Errorf("%d data requests were sent after the current user request failed", count)
	}
	if count := x.fake.requestCount("/status/"); count != syncMaxAttempts {
		t.Errorf("/status/ was requested %d times, want %d", count, syncMaxAttempts)
	}
}

func TestRunSync_RejectedSessionWithoutBrowserSession(t *testing.T) {
	x := newSyncFixture(t)
	writeFakeConfig(t, x.configPath, x.fake.server.URL, "expired-session")
	calls := 0
	refreshSession = func(string) bool {
		calls++
		return false
	}

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "auth login") {
		t.Fatalf("error = %v, want an instruction to run auth login", err)
	}
	if calls != 1 {
		t.Errorf("the browser session was read %d times, want 1", calls)
	}
	if names := x.datasetNames(t); len(names) != 0 {
		t.Errorf("datasets = %v, want none", names)
	}
}

func TestRunSync_RejectedSessionIsRefreshedFromBrowser(t *testing.T) {
	x := newSyncFixture(t)
	writeFakeConfig(t, x.configPath, x.fake.server.URL, "expired-session")
	refreshSession = func(configPath string) bool {
		cfg, err := config.Load(configPath)
		if err != nil {
			t.Errorf("loading config: %v", err)
			return false
		}
		return cfg.SaveCookies(fakeSession, fakeCSRF) == nil
	}

	x.mustSync(t)
	if ids := x.ids(t, sessionDataset(resourceCheckins)); len(ids) != 3 {
		t.Errorf("checkins = %v, want 3", ids)
	}
}

func TestRunSync_NoSessionFails(t *testing.T) {
	x := newSyncFixture(t)
	writeFakeConfig(t, x.configPath, x.fake.server.URL, "")

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "not authenticated") {
		t.Fatalf("error = %v, want a not authenticated error", err)
	}
	if count := x.fake.requestCount("/status/"); count != 0 {
		t.Errorf("%d requests were sent without a session", count)
	}
}

func TestRunSync_RefusesDatabaseOfAnotherUser(t *testing.T) {
	x := newSyncFixture(t)
	if err := x.db.ReplaceAll(sessionDataset(resourceSessionUser), []store.Record{{ID: "999", Data: json.RawMessage(`{"user_id":999}`)}}); err != nil {
		t.Fatalf("seeding session user: %v", err)
	}
	if err := x.db.ReplaceAll(sessionDataset(resourceCheckins), []store.Record{{ID: "1", Data: json.RawMessage(`{"id":1}`)}}); err != nil {
		t.Fatalf("seeding check-ins: %v", err)
	}

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "holds the data of 7Geese user 999") {
		t.Fatalf("error = %v, want a refusal that names user 999", err)
	}
	if ids := x.ids(t, sessionDataset(resourceCheckins)); strings.Join(ids, ",") != "1" {
		t.Errorf("checkins = %v, want the data of the other user unchanged", ids)
	}
	if count := x.fake.dataRequestCount(); count != 0 {
		t.Errorf("%d data requests were sent for a database of another user", count)
	}
}

func TestRunSync_PurgesLegacyDatasets(t *testing.T) {
	x := newSyncFixture(t)
	for _, name := range []string{"objectives", "teams"} {
		if err := x.db.ReplaceAll(sessionDataset(name), []store.Record{{ID: "1", Data: json.RawMessage(`{"id":1}`)}}); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	x.mustSync(t)

	names := x.datasetNames(t)
	if containsString(names, "objectives") || containsString(names, "teams") {
		t.Errorf("legacy datasets stay after a full sync: %v", names)
	}
	if !containsString(x.reporter.purged, "objectives") || !containsString(x.reporter.purged, "teams") {
		t.Errorf("purged = %v, want objectives and teams", x.reporter.purged)
	}
}

func TestRunSync_RetriesTransientFailures(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() {
		x.fake.failures["/api/v1/checkins/"] = 2
		x.fake.failures["graphql:getFinalizedOneonone"] = 1
	})
	x.mustSync(t)

	if ids := x.ids(t, sessionDataset(resourceCheckins)); len(ids) != 3 {
		t.Errorf("checkins = %v, want 3", ids)
	}
	if ids := x.ids(t, sessionDataset(resourceFinalizedOneonones)); len(ids) != 2 {
		t.Errorf("finalized_oneonones = %v, want 2", ids)
	}
}

func TestRunSync_ListCountMismatchFails(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() { x.fake.totalCountDelta["/api/v1/oneononenotes/"] = 1 })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "oneononenotes") {
		t.Fatalf("error = %v, want a failure of oneononenotes", err)
	}
	if message := x.reporter.failed["oneononenotes"]; !strings.Contains(message, "returned 2 different records but reports a total of 3") {
		t.Errorf("failure message = %q, want the record counts", message)
	}
	if names := x.datasetNames(t); containsString(names, "oneononenotes") {
		t.Errorf("a list with a wrong count was stored: %v", names)
	}
}

func TestRunSync_ReadsAnUnstableListInMorePasses(t *testing.T) {
	x := newSyncFixture(t)
	syncListPageSizes = []int{2, 3}
	x.fake.change(func() { x.fake.unstableLists["/api/v1/userprofile/"] = true })
	x.mustSync(t)

	if ids := x.ids(t, sessionDataset(resourceUserProfile)); strings.Join(ids, ",") != "100,200,300,400,500,501" {
		t.Errorf("userprofile = %v, want all 6 profiles", ids)
	}
	if count := x.fake.requestCount("/api/v1/userprofile/"); count != 5 {
		t.Errorf("the profile list was requested %d times, want 3 pages of 2 and 2 pages of 3", count)
	}
}

func TestRunSync_UnstableListThatStaysIncompleteFails(t *testing.T) {
	x := newSyncFixture(t)
	syncListPageSizes = []int{2}
	x.fake.change(func() { x.fake.unstableLists["/api/v1/userprofile/"] = true })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "userprofile") {
		t.Fatalf("error = %v, want a failure of userprofile", err)
	}
	if message := x.reporter.failed["userprofile"]; !strings.Contains(message, "returned 4 different records but reports a total of 6") {
		t.Errorf("failure message = %q, want the record counts", message)
	}
	if names := x.datasetNames(t); containsString(names, "userprofile") {
		t.Errorf("an incomplete list was stored: %v", names)
	}
}

func TestRunSync_MeetingListFailureMarksNotesAsFailed(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() { x.fake.failures["/api/v1/oneonones/"] = -1 })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "finalized_oneonones, oneonones") {
		t.Fatalf("error = %v, want the two meeting datasets", err)
	}
	if count := x.fake.requestCount("graphql:getFinalizedOneonone"); count != 0 {
		t.Errorf("%d meeting note requests were sent although the meeting list failed", count)
	}
}

func TestRunSync_MissingMeetingNotesFail(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() { delete(x.fake.meetings, 2) })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "finalized_oneonones") {
		t.Fatalf("error = %v, want a failure of finalized_oneonones", err)
	}
	if message := x.reporter.failed["finalized_oneonones"]; !strings.Contains(message, "1:1 meeting 2") {
		t.Errorf("failure message = %q, want it to name meeting 2", message)
	}
}

func TestRunSync_GetsOwnAndManagerProfileWhenTheRosterOmitsThem(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() {
		roster := x.fake.lists["/api/v1/userprofile/"]
		x.fake.single[profileURI(fakeMe)] = roster[0]
		x.fake.single[profileURI(fakeManager)] = roster[1]
		x.fake.lists["/api/v1/userprofile/"] = roster[2:]
	})
	x.mustSync(t)

	ids := x.ids(t, sessionDataset(resourceUserProfile))
	if len(ids) != 6 || !containsString(ids, "100") || !containsString(ids, "200") {
		t.Errorf("userprofile = %v, want 6 profiles with 100 and 200", ids)
	}
	if names := x.datasetNames(t); !containsString(names, "user_objectives[300]") {
		t.Errorf("the direct report was not synced: %v", names)
	}
}

func TestRunSync_JSONEvents(t *testing.T) {
	x := newSyncFixture(t)
	var out bytes.Buffer
	opts := x.options()
	opts.Reporter = &jsonSyncReporter{out: &out}
	if err := runSync(context.Background(), x.db, opts); err != nil {
		t.Fatalf("sync: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	events := map[string]int{}
	var last map[string]any
	ownerSeen := false
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		name, _ := event["event"].(string)
		events[name]++
		if event["resource"] == "user_objectives" && event["owner_id"] == float64(fakeReport) {
			ownerSeen = true
		}
		last = event
	}
	if events["sync_start"] != 15 || events["sync_complete"] != 15 || events["sync_error"] != 0 || events["sync_summary"] != 1 {
		t.Errorf("events = %v, want 15 starts, 15 completions, 1 summary", events)
	}
	if !ownerSeen {
		t.Errorf("no event names the objectives of the direct report with owner_id")
	}
	if last["event"] != "sync_summary" || last["success"] != float64(15) || last["errored"] != float64(0) || last["total_records"] != float64(323) {
		t.Errorf("last event = %v, want a summary with 15 datasets and 323 records", last)
	}
}

func TestSyncCommand_WritesJSONEventsAndFailsWithExitError(t *testing.T) {
	x := newSyncFixture(t)
	t.Setenv("API_7GEESE_CONFIG", x.configPath)
	x.db.Close()

	run := func() (string, error) {
		flags := &rootFlags{timeout: 5 * time.Second}
		cmd := newSyncCmd(flags)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs([]string{"--db", x.dbPath})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		return out.String(), err
	}

	out, err := run()
	if err != nil {
		t.Fatalf("sync command: %v", err)
	}
	if !strings.Contains(out, `"event":"sync_summary"`) {
		t.Errorf("output has no sync_summary event: %s", out)
	}

	x.fake.change(func() { x.fake.failures["/api/v1/feedback/"] = -1 })
	out, err = run()
	if err == nil || !strings.Contains(err.Error(), "peer_feedback") {
		t.Fatalf("error = %v, want a failure of peer_feedback", err)
	}
	if !strings.Contains(out, `"event":"sync_error"`) {
		t.Errorf("output has no sync_error event: %s", out)
	}
}

func TestListFeed_FailsWhenTheFeedDoesNotAdvance(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() {
		first := x.fake.notificationFeed[0]
		stuck := make([]map[string]any, 0, 120)
		for i := 0; i < 120; i++ {
			stuck = append(stuck, first)
		}
		x.fake.notificationFeed = stuck
	})

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "notifications") {
		t.Fatalf("error = %v, want a failure of notifications", err)
	}
	if message := x.reporter.failed["notifications"]; !strings.Contains(message, "does not advance") {
		t.Errorf("failure message = %q, want a feed that does not advance", message)
	}
}

func TestCanonicalID(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: `2100132`, want: "2100132"},
		{raw: `"badge:12"`, want: "badge:12"},
		{raw: `2.1e6`, wantErr: true},
		{raw: `12.5`, wantErr: true},
		{raw: `null`, wantErr: true},
		{raw: `""`, wantErr: true},
		{raw: ``, wantErr: true},
		{raw: `true`, wantErr: true},
	}
	for _, c := range cases {
		got, err := canonicalID(json.RawMessage(c.raw))
		if c.wantErr {
			if err == nil {
				t.Errorf("canonicalID(%s) = %q, want an error", c.raw, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("canonicalID(%s) = %q, %v, want %q", c.raw, got, err, c.want)
		}
	}
}

func TestRunSync_ListThatIgnoresTheOffsetFails(t *testing.T) {
	x := newSyncFixture(t)
	syncListPageSizes = []int{2}
	x.fake.change(func() { x.fake.ignoreOffset["/api/v1/userprofile/"] = true })

	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), "userprofile") {
		t.Fatalf("error = %v, want a failure of userprofile", err)
	}
	if message := x.reporter.failed["userprofile"]; !strings.Contains(message, "returned 8 records but reports a total of 6") {
		t.Errorf("failure message = %q, want a list that does not obey the offset", message)
	}
	if count := x.fake.requestCount("/api/v1/userprofile/"); count != 4 {
		t.Errorf("the profile list was requested %d times, want 4", count)
	}
}

func TestRunSync_CheckinHistoryThatDoesNotAdvanceFails(t *testing.T) {
	x := newSyncFixture(t)
	x.fake.change(func() { x.fake.stuckCheckins = true })

	key := userDataset(resourceUserObjectives, fakeMe).String()
	err := x.sync()
	if err == nil || !strings.Contains(err.Error(), key) {
		t.Fatalf("error = %v, want a failure of %s", err, key)
	}
	if message := x.reporter.failed[key]; !strings.Contains(message, "check-in history does not advance") {
		t.Errorf("failure message = %q, want a check-in history that does not advance", message)
	}
}

func TestRunSync_ReviewWithMoreThanOnePageFails(t *testing.T) {
	section := func(review fakeReview) map[string]any {
		return review.Data["snapshotGroup"].(map[string]any)["sections"].(map[string]any)
	}
	items := func(review fakeReview) map[string]any {
		node := section(review)["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)
		return node["items"].(map[string]any)
	}
	answers := func(review fakeReview) map[string]any {
		return review.Data["answers"].(map[string]any)
	}
	cases := []struct {
		name   string
		mutate func(review fakeReview)
		want   string
	}{
		{"sections", func(r fakeReview) { section(r)["pageInfo"] = map[string]any{"hasNextPage": true} }, "review 71 has more sections than one request returns"},
		{"section items", func(r fakeReview) { items(r)["pageInfo"] = map[string]any{"hasNextPage": true} }, "review 71 has more section items than one request returns"},
		{"answers", func(r fakeReview) { answers(r)["pageInfo"] = map[string]any{"hasNextPage": true} }, "review 71 has more answers than one request returns"},
		{"no page information", func(r fakeReview) { delete(answers(r), "pageInfo") }, "review 71 has no page information for its answers"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x := newSyncFixture(t)
			x.fake.change(func() { c.mutate(x.fake.reviews[fakeMe][0]) })

			key := userDataset(resourceUserSnapshots, fakeMe).String()
			err := x.sync()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("error = %v, want a failure of %s", err, key)
			}
			if message := x.reporter.failed[key]; !strings.Contains(message, c.want) {
				t.Errorf("failure message = %q, want %q", message, c.want)
			}
			if names := x.datasetNames(t); containsString(names, key) {
				t.Errorf("a review that is not complete was stored: %v", names)
			}
		})
	}
}

func TestListFeed_EndOfFeed(t *testing.T) {
	type feedPage struct {
		ids  []string
		next string
	}
	cases := []struct {
		name    string
		pages   map[string]feedPage
		want    string
		wantErr string
	}{
		{
			name:  "empty first page with no next link",
			pages: map[string]feedPage{"": {nil, ""}},
			want:  "",
		},
		{
			name:  "empty page with an offset=None link",
			pages: map[string]feedPage{"": {[]string{"a"}, "2"}, "2": {nil, "None"}},
			want:  "a",
		},
		{
			name:  "last page with only known records",
			pages: map[string]feedPage{"": {[]string{"a", "b"}, "1"}, "1": {[]string{"b"}, ""}},
			want:  "a,b",
		},
		{
			name:    "empty page with a next link",
			pages:   map[string]feedPage{"": {[]string{"a"}, "2"}, "2": {nil, "3"}},
			wantErr: "page 2 is empty but has a next link",
		},
		{
			name:    "page with only known records and a next link",
			pages:   map[string]feedPage{"": {[]string{"a"}, "2"}, "2": {[]string{"a"}, "3"}},
			wantErr: "page 2 has no new records",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			isolateSyncEnvironment(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, ok := c.pages[r.URL.Query().Get("offset")]
				if !ok {
					http.Error(w, "page not expected", http.StatusBadRequest)
					return
				}
				objects := []map[string]any{}
				for _, id := range page.ids {
					objects = append(objects, map[string]any{"id": id})
				}
				var next any
				if page.next != "" {
					next = fmt.Sprintf("/api/v1/notifications/?limit=50&offset=%s", page.next)
				}
				writeFakeJSON(w, map[string]any{"meta": map[string]any{"next": next}, "objects": objects})
			}))
			defer server.Close()
			configPath := filepath.Join(t.TempDir(), "config.json")
			writeFakeConfig(t, configPath, server.URL, fakeSession)
			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatalf("loading config: %v", err)
			}
			api, err := newSyncAPI(context.Background(), cfg, 5*time.Second, 0)
			if err != nil {
				t.Fatalf("creating the API client: %v", err)
			}

			items, err := api.listFeed("/api/v1/notifications/", map[string][]string{"limit": {"50"}})
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("error = %v, want %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("listFeed: %v", err)
			}
			ids := []string{}
			for _, item := range items {
				id, err := recordID(item)
				if err != nil {
					t.Fatalf("record ID: %v", err)
				}
				ids = append(ids, id)
			}
			if got := strings.Join(ids, ","); got != c.want {
				t.Errorf("records = %q, want %q", got, c.want)
			}
		})
	}
}
