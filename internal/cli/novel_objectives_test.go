// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"7geese-cli/internal/config"
)

func TestNormalizeGQLObjective_MapsAllFields(t *testing.T) {
	node := gqlObjectiveNode{
		PK:            99,
		Name:          "Grow revenue",
		Progress:      66.6,
		Closed:        true,
		DueDatetime:   "2026-12-31T23:59:59+00:00",
		ObjectiveType: 2,
		LastCheckin: &gqlObjectiveCheckin{
			Created: "2026-07-15T08:00:00+00:00",
		},
	}

	got := normalizeGQLObjective(node)

	checks := []struct {
		field string
		want  any
	}{
		{"id", 99},
		{"name", "Grow revenue"},
		{"progress", 66.6},
		{"closed", true},
		{"due_date", "2026-12-31T23:59:59+00:00"},
		{"objective_type", 2},
		{"updated_at", "2026-07-15T08:00:00+00:00"},
	}
	for _, c := range checks {
		if got[c.field] != c.want {
			t.Errorf("normalizeGQLObjective()[%q] = %v (%T), want %v (%T)",
				c.field, got[c.field], got[c.field], c.want, c.want)
		}
	}
}

func TestNormalizeGQLObjective_UpdatedAtEmptyWhenNoCheckin(t *testing.T) {
	node := gqlObjectiveNode{PK: 1, LastCheckin: nil}
	got := normalizeGQLObjective(node)
	if v, ok := got["updated_at"]; ok && v != "" {
		t.Errorf("updated_at = %v, want absent or empty when no last checkin", v)
	}
}

// TestStoreObjective_WritesToBothResourceTypes is the core regression guard:
// after storeObjective runs, the objective must appear in both user_objectives:<id>
// (for internal sync tracking) and objectives (so MCP tools and okr health can see it).
func TestStoreObjective_WritesToBothResourceTypes(t *testing.T) {
	db := openTestStore(t)
	node := gqlObjectiveNode{PK: 555, Name: "Test OKR", Progress: 40}

	if err := storeObjective(db, 99, node); err != nil {
		t.Fatalf("storeObjective: %v", err)
	}

	// Must appear under user_objectives:99
	userRaw, err := db.Get("user_objectives:99", "555")
	if err != nil || userRaw == nil {
		t.Error("objective not found in user_objectives:99")
	}

	// Must also appear under objectives so MCP tools can read it
	objRaw, err := db.Get("objectives", "555")
	if err != nil || objRaw == nil {
		t.Fatal("objective not found in objectives resource_type — MCP tools and okr health cannot see it")
	}

	var got map[string]any
	if err := json.Unmarshal(objRaw, &got); err != nil {
		t.Fatalf("unmarshal objectives record: %v", err)
	}
	if got["id"] != float64(555) {
		t.Errorf("objectives[id] = %v, want 555", got["id"])
	}
	if got["name"] != "Test OKR" {
		t.Errorf("objectives[name] = %v, want Test OKR", got["name"])
	}
}

// TestStoreObjective_AlwaysUpdatesExisting verifies the skip-existing guard is gone:
// calling storeObjective twice on the same PK must reflect the latest data.
func TestStoreObjective_AlwaysUpdatesExisting(t *testing.T) {
	db := openTestStore(t)
	node := gqlObjectiveNode{PK: 1, Name: "Old name", Progress: 10}

	if err := storeObjective(db, 1, node); err != nil {
		t.Fatalf("first storeObjective: %v", err)
	}

	node.Progress = 80
	node.Name = "Updated name"
	if err := storeObjective(db, 1, node); err != nil {
		t.Fatalf("second storeObjective: %v", err)
	}

	raw, err := db.Get("objectives", "1")
	if err != nil || raw == nil {
		t.Fatal("objective not found after second store")
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["progress"] != float64(80) {
		t.Errorf("progress = %v after update, want 80 — skip-existing guard must not block updates", got["progress"])
	}
	if got["name"] != "Updated name" {
		t.Errorf("name = %v after update, want 'Updated name'", got["name"])
	}
}

// newTestConfig builds a minimal Config pointing at the given test server URL.
func newTestConfig(baseURL string) *config.Config {
	return &config.Config{
		BaseURL:           baseURL,
		SevengeeseSession: "test-session",
		SevengeeseCSRF:    "test-csrf",
	}
}

// TestFetchContextObjectivesPage_ReturnsErrorOnGraphQLErrors verifies that when
// the API returns a GraphQL error response (HTTP 200 with {"errors":[...]}),
// fetchContextObjectivesPage surfaces it as a non-nil error rather than
// silently returning an empty page.  This was the root cause of user_objectives
// never being populated: the schema mismatch returned an error that was ignored.
func TestFetchContextObjectivesPage_ReturnsErrorOnGraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{
				{"message": `Variable "$userId" of type "Float!" used in position expecting type "Int".`},
			},
		})
	}))
	defer srv.Close()

	_, err := fetchContextObjectivesPage(&rootFlags{}, newTestConfig(srv.URL), "ownerOrStakeholder", 74295, "")
	if err == nil {
		t.Error("expected error when GraphQL response contains errors, got nil — silent failures hide schema mismatches")
	}
}

// TestFetchContextObjectivesPage_UsesIntTypeForUserId pins the fix for the
// schema mismatch: the query must declare $userId as Int!, not Float!.
// The 7Geese GraphQL schema expects Int for ownerOrStakeholder/follower args;
// using Float! causes a type error that was silently swallowed.
func TestFetchContextObjectivesPage_UsesIntTypeForUserId(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			capturedQuery, _ = payload["query"].(string)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"objectives": nil}})
	}))
	defer srv.Close()

	fetchContextObjectivesPage(&rootFlags{}, newTestConfig(srv.URL), "ownerOrStakeholder", 74295, "")

	if strings.Contains(capturedQuery, "Float!") {
		t.Errorf("query declares $userId as Float! — must be Int! to match 7Geese GraphQL schema")
	}
	if !strings.Contains(capturedQuery, "Int!") {
		t.Errorf("query must declare $userId as Int!, got: %q", capturedQuery)
	}
}
