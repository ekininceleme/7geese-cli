//go:build integration

// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"7geese-cli/internal/store"
)

func TestLiveSyncExportAndWeek(t *testing.T) {
	ctx := context.Background()
	db, err := store.OpenWithContext(ctx, filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	defer db.Close()

	opts := syncOptions{Timeout: 30 * time.Second, Reporter: &humanSyncReporter{out: os.Stderr}}
	if err := runSync(ctx, db, opts); err != nil {
		t.Fatalf("sync: %v", err)
	}

	out, err := buildExport(db, "", "", time.Now())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if out.Me.Profile.ID == 0 || out.Me.Profile.Name == "" {
		t.Errorf("the export has no profile: %+v", out.Me.Profile)
	}
	if out.Me.Oneonones == nil || out.Me.Recognitions == nil || out.Me.Checkins == nil || out.Me.Notifications == nil {
		t.Errorf("the export has no session user sections")
	}
	t.Logf("%d objectives, %d reviews, %d 1:1 meetings, %d direct reports",
		len(out.Me.Objectives), len(out.Me.Reviews), len(*out.Me.Oneonones), len(out.DirectReports))

	if _, err := buildWeekSummary(db, time.Now()); err != nil {
		t.Fatalf("week summary: %v", err)
	}
}
