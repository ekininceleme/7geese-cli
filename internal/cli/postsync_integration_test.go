//go:build integration

package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

const integrationDB = "/Users/ekin.inceleme/.local/share/7geese-cli/data.db"

func openIntegrationStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(integrationDB)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func integrationFlags() *rootFlags {
	return &rootFlags{configPath: ""}
}

func TestPostSyncProfile(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	if err := syncCurrentUserProfile(flags, db, profileID); err != nil {
		t.Fatalf("syncCurrentUserProfile: %v", err)
	}
	t.Logf("duration: %dms", time.Since(start).Milliseconds())
}

func TestPostSyncRecognitionProbe(t *testing.T) {
	flags := integrationFlags()

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}

	cfg, cfgErr := config.Load(flags.configPath)
	if cfgErr != nil {
		t.Fatalf("config: %v", cfgErr)
	}

	for _, filter := range []string{"sender", "recipient"} {
		t.Logf("probing filter=%s ...", filter)
		start := time.Now()
		u := fmt.Sprintf("%s/api/v1/recognitionbadges/?%s=%d&limit=1", cfg.BaseURL, filter, profileID)
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			t.Logf("  %s: ERROR %v (%dms)", filter, err, time.Since(start).Milliseconds())
			continue
		}
		var page struct {
			Meta struct{ TotalCount int `json:"total_count"` } `json:"meta"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		t.Logf("  %s: total_count=%d (%dms)", filter, page.Meta.TotalCount, time.Since(start).Milliseconds())
	}
}

func TestPostSyncRecognitionReceived(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	cfg, err := config.Load(flags.configPath)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	profileURI := fmt.Sprintf("/api/v1/userprofile/%d/", profileID)
	count, err := syncReceivedRecognition(flags, cfg, db, profileURI)
	if err != nil {
		t.Fatalf("syncReceivedRecognition: %v", err)
	}
	t.Logf("synced: %d received badges, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncRecognition(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	count, err := syncUserRecognition(flags, db, profileID)
	if err != nil {
		t.Fatalf("syncUserRecognition: %v", err)
	}
	t.Logf("synced: %d badges, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncMeetings(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	start := time.Now()
	count, err := syncFinalizedMeetings(flags, db, false)
	if err != nil {
		t.Fatalf("syncFinalizedMeetings: %v", err)
	}
	t.Logf("synced: %d meetings, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncMeetingsFull(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	start := time.Now()
	count, err := syncFinalizedMeetings(flags, db, true)
	if err != nil {
		t.Fatalf("syncFinalizedMeetings (full): %v", err)
	}
	t.Logf("synced: %d meetings, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncObjectives(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	count, err := syncUserObjectives(flags, db, profileID, false)
	if err != nil {
		t.Fatalf("syncUserObjectives: %v", err)
	}
	t.Logf("synced: %d objectives, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncSnapshots(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	count, err := syncUserSnapshots(flags, db, profileID, false)
	if err != nil {
		t.Fatalf("syncUserSnapshots: %v", err)
	}
	t.Logf("synced: %d snapshots, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncSnapshotsFull(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	count, err := syncUserSnapshots(flags, db, profileID, true)
	if err != nil {
		t.Fatalf("syncUserSnapshots (force): %v", err)
	}
	t.Logf("force-synced: %d snapshots, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncObjectivesFull(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	t.Logf("profileID = %d", profileID)

	start := time.Now()
	count, err := syncUserObjectives(flags, db, profileID, true)
	if err != nil {
		t.Fatalf("syncUserObjectives (force): %v", err)
	}
	t.Logf("force-synced: %d objectives, duration: %dms", count, time.Since(start).Milliseconds())
}

func TestPostSyncWhoami(t *testing.T) {
	flags := integrationFlags()

	start := time.Now()
	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("fetchCurrentUserID: %v", err)
	}
	t.Logf("profileID = %d, duration: %dms", profileID, time.Since(start).Milliseconds())
}

// TestPostSyncAll runs every post-sync step in sequence and prints a summary table.
func TestPostSyncAll(t *testing.T) {
	flags := integrationFlags()
	db := openIntegrationStore(t)

	profileID, err := fetchCurrentUserID(flags)
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}

	type result struct {
		name     string
		detail   string
		duration time.Duration
		err      error
	}
	var results []result

	run := func(name string, fn func() (string, error)) {
		start := time.Now()
		detail, err := fn()
		results = append(results, result{name, detail, time.Since(start), err})
	}

	run("profile", func() (string, error) {
		err := syncCurrentUserProfile(flags, db, profileID)
		return "self + manager", err
	})
	run("recognition", func() (string, error) {
		n, err := syncUserRecognition(flags, db, profileID)
		return fmt.Sprintf("%d badges", n), err
	})
	run("meetings", func() (string, error) {
		n, err := syncFinalizedMeetings(flags, db, false)
		return fmt.Sprintf("%d meetings", n), err
	})
	run("objectives", func() (string, error) {
		n, err := syncUserObjectives(flags, db, profileID, false)
		return fmt.Sprintf("%d objectives", n), err
	})

	t.Log("\n--- post-sync timing summary ---")
	for _, r := range results {
		if r.err != nil {
			t.Logf("  %-14s ERROR: %v (%dms)", r.name, r.err, r.duration.Milliseconds())
		} else {
			t.Logf("  %-14s %-20s %dms", r.name, r.detail, r.duration.Milliseconds())
		}
	}
}
