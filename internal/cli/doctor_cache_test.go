// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"7geese-cli/internal/store"
)

func TestCollectCacheReport_NamesDatasetsWithOwners(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	db, err := store.OpenWithContext(context.Background(), defaultDBPath("7geese-cli"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	one := []store.Record{{ID: "1", Data: json.RawMessage(`{"id":1}`)}}
	two := append([]store.Record{{ID: "2", Data: json.RawMessage(`{"id":2}`)}}, one...)
	if err := db.ReplaceAll(sessionDataset(resourceCheckins), two); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := db.ReplaceAll(userDataset(resourceUserObjectives, 300), one); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing: %v", err)
	}

	report := collectCacheReport(context.Background(), "")
	if report["status"] != "fresh" {
		t.Fatalf("status = %v (error %v), want fresh", report["status"], report["error"])
	}
	resources, ok := report["resources"].([]map[string]any)
	if !ok || len(resources) != 2 {
		t.Fatalf("resources = %v, want 2 datasets", report["resources"])
	}
	rows := map[string]any{}
	for _, resource := range resources {
		rows[resource["type"].(string)] = resource["rows"]
	}
	if rows["checkins"] != int64(2) || rows["user_objectives[300]"] != int64(1) {
		t.Errorf("rows = %v, want checkins 2 and user_objectives[300] 1", rows)
	}
}

func TestRenderCacheReport_ShowsTheError(t *testing.T) {
	var out bytes.Buffer
	renderCacheReport(&out, map[string]any{"status": "error", "error": "database x has schema version 2"})
	if !strings.Contains(out.String(), "error: database x has schema version 2") {
		t.Errorf("output = %q, want the error text", out.String())
	}
}
