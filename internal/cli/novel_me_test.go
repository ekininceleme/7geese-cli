// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"7geese-cli/internal/store"
)

func TestBuildWeekSummary(t *testing.T) {
	x := syncedFixture(t)
	summary, err := buildWeekSummary(x.db, fakeNow)
	if err != nil {
		t.Fatalf("week summary: %v", err)
	}

	if summary.WeekOf != "2026-06-14" {
		t.Errorf("week_of = %q, want 2026-06-14", summary.WeekOf)
	}
	if len(summary.Checkins) != 1 || summary.Checkins[0] != (weekCheckin{ID: "6001", Message: "this week", CreatedAt: "2026-06-16T09:00:00.123456+00:00"}) {
		t.Errorf("checkins = %+v", summary.Checkins)
	}
	if len(summary.StaleOKRs) != 1 || summary.StaleOKRs[0] != (weekObjective{ID: "12", Name: "Grow the team", Progress: 40}) {
		t.Errorf("okrs = %+v, want only objective 12", summary.StaleOKRs)
	}
	if len(summary.Upcoming1on1s) != 1 || summary.Upcoming1on1s[0] != (weekMeeting{ID: "3", Target: "Blake Chen", ScheduledDate: "2026-06-20T10:00:00+00:00"}) {
		t.Errorf("1:1s = %+v, want only meeting 3", summary.Upcoming1on1s)
	}
}

func TestBuildWeekSummary_EmptyListsAndMissingData(t *testing.T) {
	x := syncedFixture(t)
	summary, err := buildWeekSummary(x.db, fakeNow.AddDate(1, 0, 0))
	if err != nil {
		t.Fatalf("week summary: %v", err)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}
	for _, want := range []string{`"checkins_this_week":[]`, `"upcoming_1on1s":[]`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("JSON has no %s: %s", want, encoded)
		}
	}
	if len(summary.StaleOKRs) != 2 {
		t.Errorf("okrs = %+v, want objectives 12 and 11", summary.StaleOKRs)
	}

	if _, err := x.db.PurgeExcept([]store.DatasetKey{sessionDataset(resourceSessionUser), sessionDataset(resourceCheckins)}); err != nil {
		t.Fatalf("purging: %v", err)
	}
	if _, err := buildWeekSummary(x.db, fakeNow); err == nil || !strings.Contains(err.Error(), "is not synced; run '7geese-cli sync'") {
		t.Errorf("error = %v, want an instruction to run sync", err)
	}

	empty := newSyncFixture(t)
	if _, err := buildWeekSummary(empty.db, fakeNow); err == nil || !strings.Contains(err.Error(), "run '7geese-cli sync'") {
		t.Errorf("error = %v, want an instruction to run sync", err)
	}
}

func TestMeWeekCommand_JSON(t *testing.T) {
	x := syncedFixture(t)
	cmd := newMeWeekCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--db", x.dbPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("me week: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out.String())
	}
	for _, key := range []string{"week_of", "synced_at", "checkins_this_week", "okrs_not_updated_this_week", "upcoming_1on1s"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("output has no key %s", key)
		}
	}
}
