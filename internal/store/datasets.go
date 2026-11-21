// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type DatasetKey struct {
	ResourceType string
	OwnerID      int
}

func (k DatasetKey) String() string {
	if k.OwnerID == SessionOwner {
		return k.ResourceType
	}
	return fmt.Sprintf("%s[%d]", k.ResourceType, k.OwnerID)
}

type Record struct {
	ID   string
	Data json.RawMessage
}

func validateRecords(key DatasetKey, records []Record) error {
	if key.ResourceType == "" {
		return errors.New("dataset has no resource type")
	}
	seen := make(map[string]bool, len(records))
	for i, record := range records {
		if record.ID == "" {
			return fmt.Errorf("%s record %d has no ID", key, i)
		}
		if seen[record.ID] {
			return fmt.Errorf("%s has two records with ID %s", key, record.ID)
		}
		seen[record.ID] = true
		if !json.Valid(record.Data) {
			return fmt.Errorf("%s record %s is not valid JSON", key, record.ID)
		}
	}
	return nil
}

func (s *Store) ReplaceAll(key DatasetKey, records []Record) error {
	if err := validateRecords(key, records); err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("starting replace of %s: %w", key, err)
	}
	defer tx.Rollback()

	if err := deleteDatasetTx(tx, key); err != nil {
		return err
	}

	now := time.Now().UTC()
	insert, err := tx.Prepare(
		`INSERT INTO resources (resource_type, owner_id, id, data, synced_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("preparing insert of %s: %w", key, err)
	}
	defer insert.Close()
	for _, record := range records {
		if _, err := insert.Exec(key.ResourceType, key.OwnerID, record.ID, string(record.Data), now, now); err != nil {
			return fmt.Errorf("inserting %s/%s: %w", key, record.ID, err)
		}
	}
	if _, err := tx.Exec(
		`INSERT INTO resources_fts (rowid, id, resource_type, content)
		 SELECT row_id, id, resource_type, data FROM resources WHERE resource_type = ? AND owner_id = ?`,
		key.ResourceType, key.OwnerID); err != nil {
		return fmt.Errorf("indexing %s: %w", key, err)
	}
	if _, err := tx.Exec(
		`INSERT INTO sync_state (resource_type, owner_id, last_cursor, last_synced_at, total_count)
		 VALUES (?, ?, '', ?, ?)
		 ON CONFLICT(resource_type, owner_id) DO UPDATE SET last_cursor = excluded.last_cursor,
		 last_synced_at = excluded.last_synced_at, total_count = excluded.total_count`,
		key.ResourceType, key.OwnerID, now, len(records)); err != nil {
		return fmt.Errorf("recording sync state of %s: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing replace of %s: %w", key, err)
	}
	return nil
}

func deleteDatasetTx(tx *sql.Tx, key DatasetKey) error {
	if _, err := tx.Exec(
		`DELETE FROM resources_fts WHERE rowid IN (SELECT row_id FROM resources WHERE resource_type = ? AND owner_id = ?)`,
		key.ResourceType, key.OwnerID); err != nil {
		return fmt.Errorf("removing search index entries of %s: %w", key, err)
	}
	if _, err := tx.Exec(
		`DELETE FROM resources WHERE resource_type = ? AND owner_id = ?`,
		key.ResourceType, key.OwnerID); err != nil {
		return fmt.Errorf("removing rows of %s: %w", key, err)
	}
	return nil
}

func (s *Store) GetRecord(key DatasetKey, id string) (json.RawMessage, bool, error) {
	var data string
	err := s.db.QueryRow(
		`SELECT data FROM resources WHERE resource_type = ? AND owner_id = ? AND id = ?`,
		key.ResourceType, key.OwnerID, id,
	).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading %s/%s: %w", key, id, err)
	}
	return json.RawMessage(data), true, nil
}

func (s *Store) RecordIDs(key DatasetKey) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT id FROM resources WHERE resource_type = ? AND owner_id = ? ORDER BY id`,
		key.ResourceType, key.OwnerID)
	if err != nil {
		return nil, fmt.Errorf("listing records of %s: %w", key, err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning record ID of %s: %w", key, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating records of %s: %w", key, err)
	}
	return ids, nil
}

func (s *Store) DatasetSyncedAt(key DatasetKey) (time.Time, bool, error) {
	var syncedAt sql.NullTime
	err := s.db.QueryRow(
		`SELECT last_synced_at FROM sync_state WHERE resource_type = ? AND owner_id = ?`,
		key.ResourceType, key.OwnerID,
	).Scan(&syncedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading sync state of %s: %w", key, err)
	}
	if !syncedAt.Valid {
		return time.Time{}, false, nil
	}
	return syncedAt.Time, true, nil
}

func (s *Store) Datasets() ([]DatasetKey, error) {
	rows, err := s.db.Query(
		`SELECT resource_type, owner_id FROM resources
		 UNION
		 SELECT resource_type, owner_id FROM sync_state
		 ORDER BY 1, 2`)
	if err != nil {
		return nil, fmt.Errorf("listing datasets: %w", err)
	}
	defer rows.Close()
	var keys []DatasetKey
	for rows.Next() {
		var key DatasetKey
		if err := rows.Scan(&key.ResourceType, &key.OwnerID); err != nil {
			return nil, fmt.Errorf("scanning dataset: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating datasets: %w", err)
	}
	return keys, nil
}

func (s *Store) PurgeExcept(keep []DatasetKey) ([]DatasetKey, error) {
	existing, err := s.Datasets()
	if err != nil {
		return nil, err
	}
	kept := make(map[DatasetKey]bool, len(keep))
	for _, key := range keep {
		kept[key] = true
	}
	var purged []DatasetKey
	for _, key := range existing {
		if !kept[key] {
			purged = append(purged, key)
		}
	}
	if len(purged) == 0 {
		return nil, nil
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("starting purge: %w", err)
	}
	defer tx.Rollback()
	for _, key := range purged {
		if err := deleteDatasetTx(tx, key); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(
			`DELETE FROM sync_state WHERE resource_type = ? AND owner_id = ?`,
			key.ResourceType, key.OwnerID); err != nil {
			return nil, fmt.Errorf("removing sync state of %s: %w", key, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing purge: %w", err)
	}
	return purged, nil
}

func (s *Store) Compact() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`INSERT INTO resources_fts (resources_fts) VALUES ('optimize')`); err != nil {
		return fmt.Errorf("optimizing search index of %s: %w", s.path, err)
	}
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return fmt.Errorf("compacting database %s: %w", s.path, err)
	}
	return nil
}
