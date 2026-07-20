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
