// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func record(id, data string) Record {
	return Record{ID: id, Data: json.RawMessage(data)}
}

func datasetIDs(t *testing.T, s *Store, key DatasetKey) []string {
	t.Helper()
	rows, err := s.DB().Query(`SELECT id FROM resources WHERE resource_type = ? AND owner_id = ? ORDER BY id`, key.ResourceType, key.OwnerID)
	if err != nil {
		t.Fatalf("query ids: %v", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan id: %v", err)
		}
		ids = append(ids, id)
	}
	return ids
}

func searchCount(t *testing.T, s *Store, term string) int {
	t.Helper()
	results, err := s.Search(term, 50)
	if err != nil {
		t.Fatalf("search %q: %v", term, err)
	}
	return len(results)
}

func TestReplaceAll_RemovesDeletedRowsAndRefreshesEdits(t *testing.T) {
	s := openTempStore(t)
	key := DatasetKey{ResourceType: "oneonones", OwnerID: SessionOwner}

	if err := s.ReplaceAll(key, []Record{
		record("1", `{"id":1,"note":"alpha"}`),
		record("2", `{"id":2,"note":"bravo"}`),
	}); err != nil {
		t.Fatalf("first replace: %v", err)
	}
	if err := s.ReplaceAll(key, []Record{
		record("2", `{"id":2,"note":"charlie"}`),
		record("3", `{"id":3,"note":"delta"}`),
	}); err != nil {
		t.Fatalf("second replace: %v", err)
	}

	if got := strings.Join(datasetIDs(t, s, key), ","); got != "2,3" {
		t.Fatalf("ids = %s, want 2,3", got)
	}
	data, found, err := s.GetRecord(key, "2")
	if err != nil || !found {
		t.Fatalf("GetRecord: found=%v err=%v", found, err)
	}
	if !strings.Contains(string(data), "charlie") {
		t.Fatalf("record 2 = %s, want the edited value", data)
	}
	for term, want := range map[string]int{"alpha": 0, "bravo": 0, "charlie": 1, "delta": 1} {
		if got := searchCount(t, s, term); got != want {
			t.Errorf("search %q = %d results, want %d", term, got, want)
		}
	}
	syncedAt, found, err := s.DatasetSyncedAt(key)
	if err != nil || !found || syncedAt.IsZero() {
		t.Fatalf("DatasetSyncedAt: at=%v found=%v err=%v", syncedAt, found, err)
	}
	_, _, count, err := s.GetSyncState("oneonones")
	if err != nil || count != 2 {
		t.Fatalf("sync state count = %d err=%v, want 2", count, err)
	}
}

func TestReplaceAll_EmptyListClearsDatasetAndRecordsSync(t *testing.T) {
	s := openTempStore(t)
	key := DatasetKey{ResourceType: "feedbackrequest", OwnerID: SessionOwner}
	if err := s.ReplaceAll(key, []Record{record("1", `{"id":1}`)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := s.ReplaceAll(key, nil); err != nil {
		t.Fatalf("replace with empty list: %v", err)
	}
	if ids := datasetIDs(t, s, key); len(ids) != 0 {
		t.Fatalf("ids = %v, want none", ids)
	}
	if _, found, err := s.DatasetSyncedAt(key); err != nil || !found {
		t.Fatalf("DatasetSyncedAt: found=%v err=%v, want a sync record for an empty dataset", found, err)
	}
}

func TestReplaceAll_KeepsOwnersApart(t *testing.T) {
	s := openTempStore(t)
	first := DatasetKey{ResourceType: "user_objectives", OwnerID: 11}
	second := DatasetKey{ResourceType: "user_objectives", OwnerID: 22}
	if err := s.ReplaceAll(first, []Record{record("7", `{"pk":7,"name":"first"}`)}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := s.ReplaceAll(second, []Record{record("7", `{"pk":7,"name":"second"}`), record("8", `{"pk":8}`)}); err != nil {
		t.Fatalf("second: %v", err)
	}
	if err := s.ReplaceAll(second, []Record{record("8", `{"pk":8}`)}); err != nil {
		t.Fatalf("second again: %v", err)
	}
	if got := strings.Join(datasetIDs(t, s, first), ","); got != "7" {
		t.Fatalf("owner 11 ids = %s, want 7", got)
	}
	if got := strings.Join(datasetIDs(t, s, second), ","); got != "8" {
		t.Fatalf("owner 22 ids = %s, want 8", got)
	}
}

func TestReplaceAll_RejectsBadRecordsAndKeepsOldData(t *testing.T) {
	s := openTempStore(t)
	key := DatasetKey{ResourceType: "checkins", OwnerID: SessionOwner}
	if err := s.ReplaceAll(key, []Record{record("1", `{"id":1}`)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cases := map[string][]Record{
		"empty id":     {record("", `{"id":2}`)},
		"duplicate id": {record("2", `{"id":2}`), record("2", `{"id":2}`)},
		"invalid json": {record("2", `{"id":`)},
	}
	for name, records := range cases {
		if err := s.ReplaceAll(key, records); err == nil {
			t.Errorf("%s: ReplaceAll succeeded, want an error", name)
		}
	}
	if got := strings.Join(datasetIDs(t, s, key), ","); got != "1" {
		t.Fatalf("ids after rejected replaces = %s, want 1", got)
	}
}

func TestReplaceAll_FailureAfterTheDeleteKeepsOldData(t *testing.T) {
	s := openTempStore(t)
	key := DatasetKey{ResourceType: "checkins", OwnerID: SessionOwner}
	if err := s.ReplaceAll(key, []Record{record("1", `{"id":1,"word":"oldword"}`), record("2", `{"id":2,"word":"oldword"}`)}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, found, err := s.DatasetSyncedAt(key)
	if err != nil || !found {
		t.Fatalf("sync state after seed: found=%v err=%v", found, err)
	}
	if _, err := s.DB().Exec(`CREATE TRIGGER reject_record_4 BEFORE INSERT ON resources WHEN NEW.id = '4'
		BEGIN SELECT RAISE(ABORT, 'rejected by the test'); END`); err != nil {
		t.Fatalf("creating trigger: %v", err)
	}

	err = s.ReplaceAll(key, []Record{record("3", `{"id":3,"word":"newword"}`), record("4", `{"id":4,"word":"newword"}`)})
	if err == nil || !strings.Contains(err.Error(), "rejected by the test") {
		t.Fatalf("error = %v, want the trigger failure", err)
	}
	if got := strings.Join(datasetIDs(t, s, key), ","); got != "1,2" {
		t.Errorf("ids after the failed replace = %s, want 1,2", got)
	}
	if got := searchCount(t, s, "oldword"); got != 2 {
		t.Errorf("search for old rows = %d hits, want 2", got)
	}
	if got := searchCount(t, s, "newword"); got != 0 {
		t.Errorf("search for rolled back rows = %d hits, want 0", got)
	}
	after, found, err := s.DatasetSyncedAt(key)
	if err != nil || !found || !after.Equal(before) {
		t.Errorf("sync state after the failed replace = %v (found=%v err=%v), want %v", after, found, err, before)
	}
}

func TestPurgeExcept_RemovesOtherDatasets(t *testing.T) {
	s := openTempStore(t)
	keep := DatasetKey{ResourceType: "oneonones", OwnerID: SessionOwner}
	stale := DatasetKey{ResourceType: "objectives", OwnerID: SessionOwner}
	oldOwner := DatasetKey{ResourceType: "user_objectives", OwnerID: 99}
	for _, key := range []DatasetKey{keep, stale, oldOwner} {
		if err := s.ReplaceAll(key, []Record{record("1", `{"id":1,"word":"zulu"}`)}); err != nil {
			t.Fatalf("seed %s: %v", key, err)
		}
	}
	if err := s.SaveSyncState("userprofile:self", "", 1); err != nil {
		t.Fatalf("seed legacy sync state: %v", err)
	}

	purged, err := s.PurgeExcept([]DatasetKey{keep})
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if len(purged) != 3 {
		t.Fatalf("purged = %v, want 3 datasets", purged)
	}
	remaining, err := s.Datasets()
	if err != nil {
		t.Fatalf("datasets: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != keep {
		t.Fatalf("remaining = %v, want only %v", remaining, keep)
	}
	if got := searchCount(t, s, "zulu"); got != 1 {
		t.Fatalf("search hits after purge = %d, want 1", got)
	}
}

func createVersion2Database(t *testing.T, dbPath string) {
	t.Helper()
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	defer raw.Close()
	for _, stmt := range []string{
		`CREATE TABLE resources (
			id TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			data JSON NOT NULL,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (resource_type, id)
		)`,
		`CREATE TABLE sync_state (
			resource_type TEXT PRIMARY KEY,
			last_cursor TEXT,
			last_synced_at DATETIME,
			total_count INTEGER DEFAULT 0
		)`,
		`CREATE VIRTUAL TABLE resources_fts USING fts5(id, resource_type, content, tokenize='porter unicode61')`,
		`INSERT INTO resources (id, resource_type, data) VALUES
			('501', 'user_objectives:123', '{"pk":501,"name":"Ship the migration","closed":false,"progress":40,"dueDatetime":"2026-12-31T00:00:00+00:00","startDate":"2026-01-01T00:00:00+00:00","objectiveType":1,"checkins":{"totalCount":1},"lastCheckin":{"pk":9,"created":"2026-06-01T00:00:00+00:00"}}'),
			('502', 'user_objectives:456', '{"pk":502,"name":"Report objective"}'),
			('601', 'user_snapshots:123', '{"pk":601,"startDate":"2026-01-01","endDate":"2026-03-31","workflowState":4,"snapshotGroup":{"title":"Q1"}}'),
			('1.009882e+06', 'oneonones', '{"id":1009882,"status":1,"start_datetime":"2026-05-01T10:00:00+00:00","creator":{"id":5,"user":{"first_name":"Mona","last_name":"Manager"}},"target":{"id":123,"user":{"first_name":"Una","last_name":"User"}}}'),
			('123', 'userprofile', '{"id":123,"user":{"first_name":"Una","last_name":"User","email":"una@example.com"},"position":"Engineer","hire_date":"2022-01-11","reports_to":"/api/v1/userprofile/5/"}'),
			('5', 'userprofile', '{"id":5,"user":{"first_name":"Mona","last_name":"Manager","email":"mona@example.com"},"position":null,"hire_date":null,"reports_to":null}'),
			('777', 'objectives', '{"id":777,"name":"Company objective"}')`,
		`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (424242, 'ghost', 'ghost', 'phantomword')`,
		`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count) VALUES
			('user_objectives:123', '', '2026-09-16 10:00:00+00:00', 1),
			('userprofile:self', '', '2026-09-16 10:00:00+00:00', 1),
			('oneonones', '', '2026-09-16 10:00:00+00:00', 1)`,
		`PRAGMA user_version = 2`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seed version 2 database: %v\n%s", err, stmt)
		}
	}
}

func TestMigrate_Version2KeepsRowsAndSplitsOwners(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	createVersion2Database(t, dbPath)

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open version 2 database: %v", err)
	}
	defer s.Close()

	version, err := s.SchemaVersion()
	if err != nil || version != StoreSchemaVersion {
		t.Fatalf("version = %d err=%v, want %d", version, err, StoreSchemaVersion)
	}

	type row struct {
		resourceType string
		owner        int
		id           string
	}
	rows, err := s.DB().Query(`SELECT resource_type, owner_id, id FROM resources ORDER BY resource_type, owner_id, id`)
	if err != nil {
		t.Fatalf("query rows: %v", err)
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.resourceType, &r.owner, &r.id); err != nil {
			t.Fatalf("scan row: %v", err)
		}
		got = append(got, r)
	}
	rows.Close()
	want := []row{
		{"objectives", 0, "777"},
		{"oneonones", 0, "1.009882e+06"},
		{"user_objectives", 123, "501"},
		{"user_objectives", 456, "502"},
		{"user_snapshots", 123, "601"},
		{"userprofile", 0, "123"},
		{"userprofile", 0, "5"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}

	for _, key := range []DatasetKey{{ResourceType: "user_objectives", OwnerID: 123}, {ResourceType: "oneonones", OwnerID: SessionOwner}} {
		if _, found, err := s.DatasetSyncedAt(key); err != nil || found {
			t.Errorf("sync state of %s after the migration: found=%v err=%v, want a dataset that is not synced", key, found, err)
		}
	}
	var syncStates int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sync_state`).Scan(&syncStates); err != nil || syncStates != 0 {
		t.Errorf("sync_state has %d rows after the migration (err=%v), want 0", syncStates, err)
	}
	datasets, err := s.Datasets()
	if err != nil {
		t.Fatalf("datasets: %v", err)
	}
	names := []string{}
	for _, key := range datasets {
		names = append(names, key.String())
	}
	if got := strings.Join(names, ","); got != "objectives,oneonones,user_objectives[123],user_objectives[456],user_snapshots[123],userprofile" {
		t.Errorf("datasets after the migration = %s", got)
	}

	if got := searchCount(t, s, "phantomword"); got != 0 {
		t.Errorf("stale search entry survived the migration: %d hits", got)
	}
	if got := searchCount(t, s, "migration"); got != 1 {
		t.Errorf("search for migrated row = %d hits, want 1", got)
	}
}

func TestMigrate_Version2ReopenIsStable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	createVersion2Database(t, dbPath)
	for i := 0; i < 2; i++ {
		s, err := Open(dbPath)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		var count int
		if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources`).Scan(&count); err != nil {
			t.Fatalf("count %d: %v", i, err)
		}
		if count != 7 {
			t.Fatalf("open %d: %d rows, want 7", i, count)
		}
		s.Close()
	}
}

func TestMigrate_Version1SingleKeyTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE resources (
			id TEXT PRIMARY KEY,
			resource_type TEXT NOT NULL,
			data JSON NOT NULL,
			synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO resources (id, resource_type, data) VALUES ('9', 'oneononenotes', '{"id":9}')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	raw.Close()

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if _, found, err := s.GetRecord(DatasetKey{ResourceType: "oneononenotes", OwnerID: SessionOwner}, "9"); err != nil || !found {
		t.Fatalf("record after migration: found=%v err=%v", found, err)
	}
}

func TestViews_ExposeTypedColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	createVersion2Database(t, dbPath)
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	var fullName, email string
	var managerID sql.NullInt64
	if err := s.DB().QueryRow(`SELECT full_name, email, manager_id FROM userprofile_view WHERE id = 123`).Scan(&fullName, &email, &managerID); err != nil {
		t.Fatalf("userprofile_view: %v", err)
	}
	if fullName != "Una User" || email != "una@example.com" || !managerID.Valid || managerID.Int64 != 5 {
		t.Errorf("userprofile_view = %q %q %v", fullName, email, managerID)
	}
	if err := s.DB().QueryRow(`SELECT manager_id FROM userprofile_view WHERE id = 5`).Scan(&managerID); err != nil {
		t.Fatalf("userprofile_view manager: %v", err)
	}
	if managerID.Valid {
		t.Errorf("manager_id of a profile with no manager = %v, want NULL", managerID)
	}

	var id, status, creatorID, targetID int
	var creatorName, targetName, start string
	if err := s.DB().QueryRow(`SELECT id, status, creator_id, creator_name, target_id, target_name, start_datetime FROM oneonone_view`).
		Scan(&id, &status, &creatorID, &creatorName, &targetID, &targetName, &start); err != nil {
		t.Fatalf("oneonone_view: %v", err)
	}
	if id != 1009882 || status != 1 || creatorID != 5 || creatorName != "Mona Manager" || targetID != 123 || targetName != "Una User" {
		t.Errorf("oneonone_view = %d %d %d %q %d %q", id, status, creatorID, creatorName, targetID, targetName)
	}

	var owner, objectiveID, checkinCount int
	var closed bool
	var lastCheckin string
	if err := s.DB().QueryRow(`SELECT owner_id, id, closed, last_checkin_at, checkin_count FROM objective_view WHERE owner_id = 123`).
		Scan(&owner, &objectiveID, &closed, &lastCheckin, &checkinCount); err != nil {
		t.Fatalf("objective_view: %v", err)
	}
	if owner != 123 || objectiveID != 501 || closed || lastCheckin != "2026-06-01T00:00:00+00:00" || checkinCount != 1 {
		t.Errorf("objective_view = %d %d %v %q %d", owner, objectiveID, closed, lastCheckin, checkinCount)
	}

	var reviewTitle string
	var state int
	if err := s.DB().QueryRow(`SELECT title, workflow_state FROM review_view WHERE owner_id = 123`).Scan(&reviewTitle, &state); err != nil {
		t.Fatalf("review_view: %v", err)
	}
	if reviewTitle != "Q1" || state != 4 {
		t.Errorf("review_view = %q %d", reviewTitle, state)
	}
}

func TestOpenReadOnly_RejectsOldSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	createVersion2Database(t, dbPath)
	if _, err := OpenReadOnly(dbPath); err == nil {
		t.Fatal("OpenReadOnly opened a version 2 database, want an error")
	}
}

func TestRecordIDs_ListsOneDatasetInOrder(t *testing.T) {
	s := openTempStore(t)
	mine := DatasetKey{ResourceType: "user_objectives", OwnerID: 7}
	other := DatasetKey{ResourceType: "user_objectives", OwnerID: 8}
	if err := s.ReplaceAll(mine, []Record{record("20", `{"pk":20}`), record("10", `{"pk":10}`)}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := s.ReplaceAll(other, []Record{record("30", `{"pk":30}`)}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	ids, err := s.RecordIDs(mine)
	if err != nil {
		t.Fatalf("RecordIDs: %v", err)
	}
	if len(ids) != 2 || ids[0] != "10" || ids[1] != "20" {
		t.Errorf("ids = %v, want [10 20]", ids)
	}
	ids, err = s.RecordIDs(DatasetKey{ResourceType: "user_objectives", OwnerID: 9})
	if err != nil || len(ids) != 0 {
		t.Errorf("ids of an absent dataset = %v, %v, want none", ids, err)
	}
}

func TestCompact_ReturnsFreeSpaceAndKeepsData(t *testing.T) {
	s := openTempStore(t)
	big := DatasetKey{ResourceType: "legacy"}
	kept := DatasetKey{ResourceType: "kept"}
	records := make([]Record, 0, 2000)
	for i := 0; i < 2000; i++ {
		records = append(records, record(strconv.Itoa(i), fmt.Sprintf(`{"id":%d,"text":"%s"}`, i, strings.Repeat("x", 2000))))
	}
	if err := s.ReplaceAll(big, records); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := s.ReplaceAll(kept, []Record{record("1", `{"id":1}`)}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := s.PurgeExcept([]DatasetKey{kept}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	freePages := func() int {
		var count int
		if err := s.DB().QueryRow(`PRAGMA freelist_count`).Scan(&count); err != nil {
			t.Fatalf("freelist_count: %v", err)
		}
		return count
	}
	if before := freePages(); before == 0 {
		t.Fatalf("the purge left no free pages; the test does not exercise Compact")
	}
	if err := s.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if after := freePages(); after != 0 {
		t.Errorf("free pages after Compact = %d, want 0", after)
	}
	if ids := datasetIDs(t, s, kept); len(ids) != 1 || ids[0] != "1" {
		t.Errorf("kept dataset = %v, want [1]", ids)
	}
	if n := searchCount(t, s, "id"); n != 1 {
		t.Errorf("search index has %d rows, want 1", n)
	}
}

func TestCompact_MergesTheSearchIndex(t *testing.T) {
	s := openTempStore(t)
	big := DatasetKey{ResourceType: "legacy"}
	kept := DatasetKey{ResourceType: "kept"}
	records := make([]Record, 0, 3000)
	for i := 0; i < 3000; i++ {
		words := make([]string, 0, 40)
		for j := 0; j < 40; j++ {
			words = append(words, fmt.Sprintf("term%dx%d", i, j))
		}
		records = append(records, record(strconv.Itoa(i), fmt.Sprintf(`{"id":%d,"text":"%s"}`, i, strings.Join(words, " "))))
	}
	if err := s.ReplaceAll(big, records); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := s.ReplaceAll(kept, []Record{record("1", `{"id":1,"text":"keepword"}`)}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if _, err := s.PurgeExcept([]DatasetKey{kept}); err != nil {
		t.Fatalf("purge: %v", err)
	}
	indexBytes := func() int {
		var size int
		if err := s.DB().QueryRow(`SELECT SUM(LENGTH(block)) FROM resources_fts_data`).Scan(&size); err != nil {
			t.Fatalf("index size: %v", err)
		}
		return size
	}
	before := indexBytes()
	if err := s.Compact(); err != nil {
		t.Fatalf("Compact: %v", err)
	}
	after := indexBytes()
	if after*10 > before {
		t.Errorf("search index = %d bytes after Compact, %d before; want less than a tenth", after, before)
	}
	if n := searchCount(t, s, "keepword"); n != 1 {
		t.Errorf("search for the kept row = %d hits, want 1", n)
	}
	if n := searchCount(t, s, "term5x5"); n != 0 {
		t.Errorf("search for a purged row = %d hits, want 0", n)
	}
}

func TestViews_ExposeFeedback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	key := DatasetKey{ResourceType: "feedback_requests", OwnerID: SessionOwner}
	data := json.RawMessage(`{"pk":81,"title":"Mid-Year","statusByRecipient":"COMPLETE","created":"2026-05-18T10:00:00+00:00","sentDate":"2026-05-18T10:05:00+00:00",
		"creator":{"fullName":"Mona Manager"},"numResponsesSubmitted":2,"feedbacks":{"edges":[
		{"node":{"pk":811,"submittedTimestamp":"2026-05-20T09:00:00+00:00","responder":{"profile":{"fullName":"Pat Peer"}},"answers":{"edges":[
			{"node":{"pk":1,"text":"Great work","question":{"title":"What went well?"}}},
			{"node":{"pk":2,"number":4,"question":{"title":"Rate the work"}}}]}}},
		{"node":{"pk":812,"submittedTimestamp":null,"responder":{"profile":null},"answers":{"edges":[]}}}]}}`)
	if err := s.ReplaceAll(key, []Record{{ID: "81", Data: data}, {ID: "82", Data: json.RawMessage(`{"pk":82,"title":"Draft","feedbacks":{"edges":[]}}`)},
		{ID: "83", Data: json.RawMessage(`{"pk":83,"title":"Hidden","notVisible":true}`)}}); err != nil {
		t.Fatalf("storing feedback requests: %v", err)
	}

	var id, submitted int
	var title, status, created, sent, creator string
	if err := s.DB().QueryRow(`SELECT id, title, status, created, sent_date, creator_name, responses_submitted FROM feedback_request_view WHERE id = 81`).
		Scan(&id, &title, &status, &created, &sent, &creator, &submitted); err != nil {
		t.Fatalf("feedback_request_view: %v", err)
	}
	if id != 81 || title != "Mid-Year" || status != "COMPLETE" || created != "2026-05-18T10:00:00+00:00" || sent != "2026-05-18T10:05:00+00:00" || creator != "Mona Manager" || submitted != 2 {
		t.Errorf("feedback_request_view = %d %q %q %q %q %q %d", id, title, status, created, sent, creator, submitted)
	}
	var requests int
	if err := s.DB().QueryRow(`SELECT count(*) FROM feedback_request_view`).Scan(&requests); err != nil || requests != 3 {
		t.Errorf("feedback_request_view has %d rows (err %v), want 3", requests, err)
	}
	var visible string
	if err := s.DB().QueryRow(`SELECT group_concat(id || ':' || visible, ' ') FROM (SELECT id, visible FROM feedback_request_view ORDER BY id)`).Scan(&visible); err != nil || visible != "81:1 82:1 83:0" {
		t.Errorf("visible column = %q (err %v), want 81:1 82:1 83:0", visible, err)
	}

	rows, err := s.DB().Query(`SELECT request_id, request_title, response_id, responder_name, submitted_at, question, answer_text, answer_number FROM feedback_answer_view ORDER BY question`)
	if err != nil {
		t.Fatalf("feedback_answer_view: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var requestID, responseID int
		var requestTitle, responder, submittedAt, question string
		var text sql.NullString
		var number sql.NullInt64
		if err := rows.Scan(&requestID, &requestTitle, &responseID, &responder, &submittedAt, &question, &text, &number); err != nil {
			t.Fatalf("scanning feedback_answer_view: %v", err)
		}
		got = append(got, fmt.Sprintf("%d|%s|%d|%s|%s|%s|%s|%d", requestID, requestTitle, responseID, responder, submittedAt, question, text.String, number.Int64))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading feedback_answer_view: %v", err)
	}
	want := []string{
		"81|Mid-Year|811|Pat Peer|2026-05-20T09:00:00+00:00|Rate the work||4",
		"81|Mid-Year|811|Pat Peer|2026-05-20T09:00:00+00:00|What went well?|Great work|0",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("feedback_answer_view = %v, want %v", got, want)
	}
}
