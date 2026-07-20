// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"7geese-cli/internal/store"
)

// mockSyncClient is a minimal test double for the syncResource client interface.
type mockSyncClient struct {
	calls []map[string]string // params from each Get call, in order
	resp  json.RawMessage     // returned for every call
}

func (m *mockSyncClient) Get(_ string, params map[string]string) (json.RawMessage, error) {
	captured := make(map[string]string, len(params))
	for k, v := range params {
		captured[k] = v
	}
	m.calls = append(m.calls, captured)
	return m.resp, nil
}

func (m *mockSyncClient) RateLimit() float64 { return 0 }

// oneShotDRFResponse is a valid DRF envelope with one item and no next page,
// so syncResource exits after a single API call.
func oneShotDRFResponse() json.RawMessage {
	return json.RawMessage(`{"meta":{"next":null,"total_count":1},"objects":[{"id":99999}]}`)
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestSyncResource_IncrementalResumesFromOffset is the core behaviour: when a
// previous sync completed (cursor="", count=N), the next incremental run must
// start at offset = N - backtrackBuffer, not at 0.  Without this, every run
// re-scans all pages and hits the --max-pages cap before reaching new records.
func TestSyncResource_IncrementalResumesFromOffset(t *testing.T) {
	db := openTestStore(t)
	const lastCount = 500
	if err := db.SaveSyncState("oneononenotes", "", lastCount); err != nil {
		t.Fatalf("SaveSyncState: %v", err)
	}

	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "oneononenotes", "", false, 1, map[string]string{})

	if len(client.calls) == 0 {
		t.Fatal("expected at least one API call")
	}
	got := client.calls[0]["offset"]
	// backtrackBuffer = syncBacktrackPages(2) * pageSize(100) = 200; 500-200 = 300
	want := "300"
	if got != want {
		t.Errorf("first request offset = %q, want %q (should resume near lastCount, not re-scan from 0)", got, want)
	}
}

// TestSyncResource_InProgressCursorTakesPrecedence is a regression guard: when
// an interrupted sync left a non-empty cursor, that exact cursor must be used
// rather than overridden by the lastCount-derived offset.
func TestSyncResource_InProgressCursorTakesPrecedence(t *testing.T) {
	db := openTestStore(t)
	if err := db.SaveSyncState("oneononenotes", "150", 150); err != nil {
		t.Fatalf("SaveSyncState: %v", err)
	}

	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "oneononenotes", "", false, 1, map[string]string{})

	if len(client.calls) == 0 {
		t.Fatal("expected at least one API call")
	}
	got := client.calls[0]["offset"]
	if got != "150" {
		t.Errorf("first request offset = %q, want %q (in-progress cursor must not be overridden)", got, "150")
	}
}

// TestSyncResource_CompletedCountIncludesResumeOffset verifies that after an
// offset-resumed sync completes, the saved total_count reflects the full
// approximate API total (resumeOffset + newlyFetched), not just the records
// fetched in this run. Without this, every subsequent run re-derives a
// startOffset far behind the true end of the dataset and over-fetches.
func TestSyncResource_CompletedCountIncludesResumeOffset(t *testing.T) {
	db := openTestStore(t)
	const lastCount = 19940
	if err := db.SaveSyncState("oneononenotes", "", lastCount); err != nil {
		t.Fatalf("SaveSyncState: %v", err)
	}

	// Return one item then stop, simulating "only new records since last sync".
	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "oneononenotes", "", false, 10, map[string]string{})

	_, _, savedCount, err := db.GetSyncState("oneononenotes")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	// startOffset = 19940 - 200 = 19740; fetched 1 item → want 19740 + 1 = 19741
	want := 19741
	if savedCount != want {
		t.Errorf("saved count = %d, want %d (should be resumeOffset + fetched, not just fetched)", savedCount, want)
	}
}

// TestSyncResource_ResumedFromCursorCountIncludesCursorOffset is the bug this
// fix addresses: when a sync was interrupted (cursor="17240", count=17240) and
// then resumed, the completed count was saved as just the records fetched in
// the resumed run (e.g. 100), not cursor_offset + fetched (17340). The next
// run then derived a wildly wrong startOffset and over-fetched.
func TestSyncResource_ResumedFromCursorCountIncludesCursorOffset(t *testing.T) {
	db := openTestStore(t)
	// State left by an interrupted sync: mid-pagination cursor and count so far.
	if err := db.SaveSyncState("oneononenotes", "17240", 17240); err != nil {
		t.Fatalf("SaveSyncState: %v", err)
	}

	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "oneononenotes", "", false, 10, map[string]string{})

	_, _, savedCount, err := db.GetSyncState("oneononenotes")
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	// cursor offset 17240 + 1 fetched = 17241
	want := 17241
	if savedCount != want {
		t.Errorf("saved count = %d, want %d (resumed run must add cursor offset to final count)", savedCount, want)
	}
}

// TestSyncResource_BacktrackClampsAtZero verifies that when lastCount is
// smaller than the backtrack buffer the offset clamps to zero — i.e., no
// offset param is sent — rather than going negative.
func TestSyncResource_BacktrackClampsAtZero(t *testing.T) {
	db := openTestStore(t)
	const lastCount = 50 // less than backtrackBuffer (2 pages * 100 = 200)
	if err := db.SaveSyncState("oneononenotes", "", lastCount); err != nil {
		t.Fatalf("SaveSyncState: %v", err)
	}

	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "oneononenotes", "", false, 1, map[string]string{})

	if len(client.calls) == 0 {
		t.Fatal("expected at least one API call")
	}
	if offset, ok := client.calls[0]["offset"]; ok && offset != "" {
		t.Errorf("first request offset = %q, want absent (backtrack buffer exceeds lastCount, should start from beginning)", offset)
	}
}
