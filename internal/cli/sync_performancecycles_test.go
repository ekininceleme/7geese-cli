// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
)

// TestDefaultSyncResources_ExcludesPerformanceCycles verifies that
// performancecycles is not in the default sync resource list. It must be
// synced as a post-sync user-scoped task (target=<profileID>) to avoid
// scanning the entire company's performance cycle history and timing out.
func TestDefaultSyncResources_ExcludesPerformanceCycles(t *testing.T) {
	for _, r := range defaultSyncResources() {
		if r == "performancecycles" {
			t.Errorf("defaultSyncResources() contains %q — it must be removed and synced as a user-scoped post-sync task", r)
		}
	}
}

// TestSyncResource_PerformanceCyclesPassesTargetParam verifies that when
// syncResource is called for performancecycles with a target extraParam,
// the target value is forwarded to every API request.
func TestSyncResource_PerformanceCyclesPassesTargetParam(t *testing.T) {
	db := openTestStore(t)

	client := &mockSyncClient{resp: oneShotDRFResponse()}
	syncResource(client, db, "performancecycles", "", false, 1,
		map[string]string{"target": "42"})

	if len(client.calls) == 0 {
		t.Fatal("expected at least one API call")
	}
	if got := client.calls[0]["target"]; got != "42" {
		t.Errorf("first request target = %q, want %q", got, "42")
	}
}
