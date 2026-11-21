// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

type viewDefinition struct {
	Name   string
	Select string
}

var viewDefinitions = []viewDefinition{
	{Name: "userprofile_view", Select: `
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER) AS id,
			json_extract(data,'$.user.first_name') AS first_name,
			json_extract(data,'$.user.last_name') AS last_name,
			trim(json_extract(data,'$.user.first_name') || ' ' || json_extract(data,'$.user.last_name')) AS full_name,
			json_extract(data,'$.user.email') AS email,
			json_extract(data,'$.position') AS position,
			json_extract(data,'$.hire_date') AS hire_date,
			CAST(substr(json_extract(data,'$.reports_to'), 21) AS INTEGER) AS manager_id
		FROM resources
		WHERE resource_type = 'userprofile' AND owner_id = 0`},
	{Name: "oneonone_view", Select: `
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER) AS id,
			json_extract(data,'$.start_datetime') AS start_datetime,
			json_extract(data,'$.status') AS status,
			CAST(json_extract(data,'$.creator.id') AS INTEGER) AS creator_id,
			trim(json_extract(data,'$.creator.user.first_name') || ' ' || json_extract(data,'$.creator.user.last_name')) AS creator_name,
			CAST(json_extract(data,'$.target.id') AS INTEGER) AS target_id,
			trim(json_extract(data,'$.target.user.first_name') || ' ' || json_extract(data,'$.target.user.last_name')) AS target_name
		FROM resources
		WHERE resource_type = 'oneonones' AND owner_id = 0`},
	{Name: "objective_view", Select: `
		SELECT
			owner_id,
			CAST(json_extract(data,'$.pk') AS INTEGER) AS id,
			json_extract(data,'$.name') AS name,
			json_extract(data,'$.objectiveType') AS objective_type,
			json_extract(data,'$.progress') AS progress,
			json_extract(data,'$.closed') AS closed,
			json_extract(data,'$.draft') AS draft,
			json_extract(data,'$.startDate') AS start_date,
			json_extract(data,'$.dueDatetime') AS due_datetime,
			json_extract(data,'$.completedDatetime') AS completed_datetime,
			json_extract(data,'$.lastCheckin.created') AS last_checkin_at,
			json_extract(data,'$.checkins.totalCount') AS checkin_count,
			data
		FROM resources
		WHERE resource_type = 'user_objectives'`},
	{Name: "review_view", Select: `
		SELECT
			owner_id,
			CAST(json_extract(data,'$.pk') AS INTEGER) AS id,
			json_extract(data,'$.snapshotGroup.title') AS title,
			json_extract(data,'$.startDate') AS start_date,
			json_extract(data,'$.endDate') AS end_date,
			json_extract(data,'$.workflowState') AS workflow_state,
			json_extract(data,'$.position') AS position,
			json_extract(data,'$.manager.fullName') AS manager_name,
			data
		FROM resources
		WHERE resource_type = 'user_snapshots'`},
	{Name: "recognition_given_view", Select: `
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER) AS id,
			json_extract(data,'$.created') AS created,
			trim(json_extract(data,'$.sender.user.first_name') || ' ' || json_extract(data,'$.sender.user.last_name')) AS sender_name,
			trim(json_extract(data,'$.recipient.user.first_name') || ' ' || json_extract(data,'$.recipient.user.last_name')) AS recipient_name,
			json_extract(data,'$.badge.name') AS badge,
			json_extract(data,'$.message') AS message
		FROM resources
		WHERE resource_type = 'recognitionbadges' AND owner_id = 0`},
	{Name: "recognition_received_view", Select: `
		SELECT
			json_extract(data,'$.id') AS id,
			json_extract(data,'$.published') AS published,
			json_extract(data,'$.actor.displayName') AS sender_name,
			json_extract(data,'$.target.displayName') AS recipient_name,
			json_extract(data,'$.object.badgeName') AS badge,
			json_extract(data,'$.object.content') AS message
		FROM resources
		WHERE resource_type = 'recognition_received' AND owner_id = 0`},
	{Name: "checkin_view", Select: `
		SELECT
			CAST(json_extract(data,'$.id') AS INTEGER) AS id,
			json_extract(data,'$.created') AS created,
			json_extract(data,'$.message') AS message,
			CAST(json_extract(data,'$.creator.id') AS INTEGER) AS creator_id,
			CAST(json_extract(data,'$.metadata.metadata.objective.snapshot.id') AS INTEGER) AS objective_id,
			json_extract(data,'$.metadata.metadata.objective.snapshot.name') AS objective_name,
			json_extract(data,'$.metadata.metadata.objective.snapshot.progress') AS progress
		FROM resources
		WHERE resource_type = 'checkins' AND owner_id = 0`},
	{Name: "notification_view", Select: `
		SELECT
			json_extract(data,'$.id') AS id,
			json_extract(data,'$.published') AS published,
			json_extract(data,'$.verb') AS verb,
			json_extract(data,'$.actor.displayName') AS actor_name,
			json_extract(data,'$.content') AS content
		FROM resources
		WHERE resource_type = 'notifications' AND owner_id = 0`},
	{Name: "feedback_request_view", Select: `
		SELECT
			CAST(json_extract(data,'$.pk') AS INTEGER) AS id,
			json_extract(data,'$.title') AS title,
			json_extract(data,'$.notVisible') IS NOT 1 AS visible,
			json_extract(data,'$.statusByRecipient') AS status,
			json_extract(data,'$.created') AS created,
			json_extract(data,'$.sentDate') AS sent_date,
			json_extract(data,'$.creator.fullName') AS creator_name,
			json_extract(data,'$.numResponsesSubmitted') AS responses_submitted,
			data
		FROM resources
		WHERE resource_type = 'feedback_requests' AND owner_id = 0`},
	{Name: "feedback_answer_view", Select: `
		SELECT
			CAST(json_extract(request.data,'$.pk') AS INTEGER) AS request_id,
			json_extract(request.data,'$.title') AS request_title,
			CAST(json_extract(response.value,'$.node.pk') AS INTEGER) AS response_id,
			json_extract(response.value,'$.node.responder.profile.fullName') AS responder_name,
			json_extract(response.value,'$.node.submittedTimestamp') AS submitted_at,
			json_extract(answer.value,'$.node.question.title') AS question,
			json_extract(answer.value,'$.node.text') AS answer_text,
			json_extract(answer.value,'$.node.number') AS answer_number,
			json_extract(answer.value,'$.node') AS data
		FROM resources AS request,
			json_each(request.data,'$.feedbacks.edges') AS response,
			json_each(response.value,'$.node.answers.edges') AS answer
		WHERE request.resource_type = 'feedback_requests' AND request.owner_id = 0`},
}

var legacyOwnedPrefixes = []string{"user_objectives:", "user_snapshots:"}

func legacyDatasetKey(resourceType string) DatasetKey {
	for _, prefix := range legacyOwnedPrefixes {
		if !strings.HasPrefix(resourceType, prefix) {
			continue
		}
		owner, err := strconv.Atoi(strings.TrimPrefix(resourceType, prefix))
		if err == nil && owner > 0 {
			return DatasetKey{ResourceType: strings.TrimSuffix(prefix, ":"), OwnerID: owner}
		}
	}
	return DatasetKey{ResourceType: resourceType, OwnerID: SessionOwner}
}

func resourcesTableSQL(name string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
		row_id INTEGER PRIMARY KEY,
		resource_type TEXT NOT NULL,
		owner_id INTEGER NOT NULL,
		id TEXT NOT NULL,
		data JSON NOT NULL,
		synced_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE (resource_type, owner_id, id)
	)`, name)
}

func syncStateTableSQL(name string) string {
	return fmt.Sprintf(`CREATE TABLE %s (
		resource_type TEXT NOT NULL,
		owner_id INTEGER NOT NULL,
		last_cursor TEXT,
		last_synced_at DATETIME,
		total_count INTEGER DEFAULT 0,
		PRIMARY KEY (resource_type, owner_id)
	)`, name)
}

func applySchema(ctx context.Context, conn *sql.Conn) error {
	for _, view := range viewDefinitions {
		if _, err := conn.ExecContext(ctx, "DROP VIEW IF EXISTS "+view.Name); err != nil {
			return fmt.Errorf("dropping view %s: %w", view.Name, err)
		}
	}
	resourcesRebuilt, err := migrateResources(ctx, conn)
	if err != nil {
		return err
	}
	if err := migrateSyncState(ctx, conn); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_resources_type ON resources(resource_type, owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_resources_synced ON resources(synced_at)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS resources_fts USING fts5(
			id, resource_type, content, tokenize='porter unicode61'
		)`,
	} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("applying schema statement: %w", err)
		}
	}
	if resourcesRebuilt {
		if _, err := conn.ExecContext(ctx, `DELETE FROM resources_fts`); err != nil {
			return fmt.Errorf("clearing search index: %w", err)
		}
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO resources_fts (rowid, id, resource_type, content)
			 SELECT row_id, id, resource_type, data FROM resources`); err != nil {
			return fmt.Errorf("rebuilding search index: %w", err)
		}
	}
	for _, view := range viewDefinitions {
		if _, err := conn.ExecContext(ctx, "CREATE VIEW "+view.Name+" AS "+view.Select); err != nil {
			return fmt.Errorf("creating view %s: %w", view.Name, err)
		}
	}
	return nil
}

func tableColumns(ctx context.Context, conn *sql.Conn, table string) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info("%s")`, table))
	if err != nil {
		return nil, fmt.Errorf("table_info %s: %w", table, err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan table_info %s: %w", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating table_info %s: %w", table, err)
	}
	return columns, nil
}

func distinctResourceTypes(ctx context.Context, conn *sql.Conn, table string) ([]string, error) {
	rows, err := conn.QueryContext(ctx, fmt.Sprintf(`SELECT DISTINCT resource_type FROM "%s"`, table))
	if err != nil {
		return nil, fmt.Errorf("listing resource types of %s: %w", table, err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var resourceType string
		if err := rows.Scan(&resourceType); err != nil {
			return nil, fmt.Errorf("scanning resource type of %s: %w", table, err)
		}
		types = append(types, resourceType)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating resource types of %s: %w", table, err)
	}
	return types, nil
}

func migrateResources(ctx context.Context, conn *sql.Conn) (bool, error) {
	columns, err := tableColumns(ctx, conn, "resources")
	if err != nil {
		return false, err
	}
	if len(columns) == 0 {
		if _, err := conn.ExecContext(ctx, resourcesTableSQL("resources")); err != nil {
			return false, fmt.Errorf("creating resources table: %w", err)
		}
		return false, nil
	}
	if columns["owner_id"] {
		return false, nil
	}

	types, err := distinctResourceTypes(ctx, conn, "resources")
	if err != nil {
		return false, err
	}
	if _, err := conn.ExecContext(ctx, resourcesTableSQL("resources_v3")); err != nil {
		return false, fmt.Errorf("creating resources_v3 table: %w", err)
	}
	for _, resourceType := range types {
		key := legacyDatasetKey(resourceType)
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO resources_v3 (resource_type, owner_id, id, data, synced_at, updated_at)
			 SELECT ?, ?, id, data, COALESCE(synced_at, CURRENT_TIMESTAMP), COALESCE(updated_at, CURRENT_TIMESTAMP)
			 FROM resources WHERE resource_type = ?`,
			key.ResourceType, key.OwnerID, resourceType); err != nil {
			return false, fmt.Errorf("copying %s rows to resources_v3: %w", resourceType, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE resources`); err != nil {
		return false, fmt.Errorf("dropping old resources table: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `ALTER TABLE resources_v3 RENAME TO resources`); err != nil {
		return false, fmt.Errorf("renaming resources_v3: %w", err)
	}
	return true, nil
}

func migrateSyncState(ctx context.Context, conn *sql.Conn) error {
	columns, err := tableColumns(ctx, conn, "sync_state")
	if err != nil {
		return err
	}
	if len(columns) == 0 {
		if _, err := conn.ExecContext(ctx, syncStateTableSQL("sync_state")); err != nil {
			return fmt.Errorf("creating sync_state table: %w", err)
		}
		return nil
	}
	if columns["owner_id"] {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `DROP TABLE sync_state`); err != nil {
		return fmt.Errorf("dropping old sync_state table: %w", err)
	}
	if _, err := conn.ExecContext(ctx, syncStateTableSQL("sync_state")); err != nil {
		return fmt.Errorf("creating sync_state table: %w", err)
	}
	return nil
}
