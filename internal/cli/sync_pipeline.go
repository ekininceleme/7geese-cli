// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

const (
	resourceSessionUser        = "session_user"
	resourceUserProfile        = "userprofile"
	resourceOneonones          = "oneonones"
	resourceFinalizedOneonones = "finalized_oneonones"
	resourceOneononeNotes      = "oneononenotes"
	resourceFeedbackRequests   = "feedback_requests"
	resourcePerformanceCycles  = "performancecycles"
	resourceRecognitionGiven   = "recognitionbadges"
	resourceRecognitionGot     = "recognition_received"
	resourceCheckins           = "checkins"
	resourceNotifications      = "notifications"
	resourceUserObjectives     = "user_objectives"
	resourceUserSnapshots      = "user_snapshots"

	oneononeStatusCompleted = 1
	feedPageSize            = 50
)

var refreshSession = TryRefreshSession

func sessionDataset(resourceType string) store.DatasetKey {
	return store.DatasetKey{ResourceType: resourceType, OwnerID: store.SessionOwner}
}

func userDataset(resourceType string, profileID int) store.DatasetKey {
	return store.DatasetKey{ResourceType: resourceType, OwnerID: profileID}
}

func profileURI(profileID int) string {
	return fmt.Sprintf("/api/v1/userprofile/%d/", profileID)
}

func profileIDFromURI(uri string) (int, error) {
	const prefix = "/api/v1/userprofile/"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, "/") {
		return 0, fmt.Errorf("%q is not a user profile URI", uri)
	}
	id, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(uri, prefix), "/"))
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%q is not a user profile URI", uri)
	}
	return id, nil
}

type syncReporter interface {
	Start(key store.DatasetKey)
	Done(key store.DatasetKey, count int, elapsed time.Duration)
	Failed(key store.DatasetKey, err error, elapsed time.Duration)
	Purged(key store.DatasetKey)
	Summary(records, succeeded, failed int, elapsed time.Duration)
}

type jsonSyncReporter struct {
	mu  sync.Mutex
	out io.Writer
}

func (r *jsonSyncReporter) emit(event string, key *store.DatasetKey, fields map[string]any) {
	fields["event"] = event
	if key != nil {
		fields["resource"] = key.ResourceType
		if key.OwnerID != store.SessionOwner {
			fields["owner_id"] = key.OwnerID
		}
	}
	line, err := json.Marshal(fields)
	if err != nil {
		panic(fmt.Sprintf("encoding sync event %s: %v", event, err))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.out, string(line))
}

func (r *jsonSyncReporter) Start(key store.DatasetKey) {
	r.emit("sync_start", &key, map[string]any{})
}

func (r *jsonSyncReporter) Done(key store.DatasetKey, count int, elapsed time.Duration) {
	r.emit("sync_complete", &key, map[string]any{"total": count, "duration_ms": elapsed.Milliseconds()})
}

func (r *jsonSyncReporter) Failed(key store.DatasetKey, err error, elapsed time.Duration) {
	r.emit("sync_error", &key, map[string]any{"message": err.Error(), "duration_ms": elapsed.Milliseconds()})
}

func (r *jsonSyncReporter) Purged(key store.DatasetKey) {
	r.emit("sync_purge", &key, map[string]any{})
}

func (r *jsonSyncReporter) Summary(records, succeeded, failed int, elapsed time.Duration) {
	r.emit("sync_summary", nil, map[string]any{
		"total_records": records,
		"resources":     succeeded + failed,
		"success":       succeeded,
		"errored":       failed,
		"duration_ms":   elapsed.Milliseconds(),
	})
}

type humanSyncReporter struct {
	mu  sync.Mutex
	out io.Writer
}

func (r *humanSyncReporter) line(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.out, format+"\n", args...)
}

func (r *humanSyncReporter) Start(key store.DatasetKey) {}

func (r *humanSyncReporter) Done(key store.DatasetKey, count int, elapsed time.Duration) {
	r.line("  %s: %d synced (%.1fs)", key, count, elapsed.Seconds())
}

func (r *humanSyncReporter) Failed(key store.DatasetKey, err error, elapsed time.Duration) {
	r.line("  %s: error: %v (%.1fs)", key, err, elapsed.Seconds())
}

func (r *humanSyncReporter) Purged(key store.DatasetKey) {
	r.line("  %s: removed, it is not part of the synced data", key)
}

func (r *humanSyncReporter) Summary(records, succeeded, failed int, elapsed time.Duration) {
	if failed > 0 {
		r.line("Sync failed: %d of %d datasets failed, %d records synced (%.1fs)", failed, succeeded+failed, records, elapsed.Seconds())
		return
	}
	r.line("Sync complete: %d records in %d datasets (%.1fs)", records, succeeded, elapsed.Seconds())
}

type syncOptions struct {
	ConfigPath string
	Timeout    time.Duration
	RateLimit  float64
	Reporter   syncReporter
}

func syncOptionsFromFlags(flags *rootFlags, reporter syncReporter) syncOptions {
	return syncOptions{
		ConfigPath: flags.configPath,
		Timeout:    flags.timeout,
		RateLimit:  flags.rateLimit,
		Reporter:   reporter,
	}
}

func openSyncSession(ctx context.Context, opts syncOptions) (*syncAPI, int, error) {
	connect := func() (*syncAPI, int, error) {
		cfg, err := config.Load(opts.ConfigPath)
		if err != nil {
			return nil, 0, fmt.Errorf("loading config: %w", err)
		}
		api, err := newSyncAPI(ctx, cfg, opts.Timeout, opts.RateLimit)
		if err != nil {
			return nil, 0, err
		}
		profileID, err := api.currentUserID()
		return api, profileID, err
	}

	api, profileID, err := connect()
	if err == nil {
		return api, profileID, nil
	}
	if !isSessionRejected(err) {
		return nil, 0, fmt.Errorf("reading the current user from 7Geese: %w", err)
	}
	if !refreshSession(opts.ConfigPath) {
		return nil, 0, fmt.Errorf("the 7Geese session is not valid and a new session was not found in the browser; run '7geese-cli auth login' (%w)", err)
	}
	api, profileID, err = connect()
	if err != nil {
		return nil, 0, fmt.Errorf("reading the current user from 7Geese with the new browser session: %w", err)
	}
	return api, profileID, nil
}

func storedSessionUser(db *store.Store) (int, bool, error) {
	ids, err := db.RecordIDs(sessionDataset(resourceSessionUser))
	if err != nil {
		return 0, false, err
	}
	if len(ids) == 0 {
		return 0, false, nil
	}
	if len(ids) > 1 {
		return 0, false, fmt.Errorf("the local database names %d session users (%s); delete the database file and run sync again", len(ids), strings.Join(ids, ", "))
	}
	id, err := strconv.Atoi(ids[0])
	if err != nil || id <= 0 {
		return 0, false, fmt.Errorf("the local database has an invalid session user ID %q", ids[0])
	}
	return id, true, nil
}

func claimDatabase(db *store.Store, profileID int) error {
	owner, found, err := storedSessionUser(db)
	if err != nil {
		return err
	}
	if found && owner != profileID {
		return fmt.Errorf("the local database %s holds the data of 7Geese user %d, but the session is user %d; use --db with a different path or delete the database file", db.Path(), owner, profileID)
	}
	if found {
		return nil
	}
	data, err := json.Marshal(map[string]int{"user_id": profileID})
	if err != nil {
		return fmt.Errorf("encoding session user: %w", err)
	}
	return db.ReplaceAll(sessionDataset(resourceSessionUser), []store.Record{{ID: strconv.Itoa(profileID), Data: data}})
}

type syncRun struct {
	api       *syncAPI
	db        *store.Store
	reporter  syncReporter
	profileID int

	mu      sync.Mutex
	synced  []store.DatasetKey
	failed  []store.DatasetKey
	records int
}

func (r *syncRun) fail(key store.DatasetKey, err error, elapsed time.Duration) {
	r.mu.Lock()
	r.failed = append(r.failed, key)
	r.mu.Unlock()
	r.reporter.Failed(key, err, elapsed)
}

func (r *syncRun) dataset(key store.DatasetKey, fetch func() ([]store.Record, error)) bool {
	started := time.Now()
	r.reporter.Start(key)
	records, err := fetch()
	if err == nil {
		err = r.db.ReplaceAll(key, records)
	}
	if err != nil {
		r.fail(key, err, time.Since(started))
		return false
	}
	r.mu.Lock()
	r.synced = append(r.synced, key)
	r.records += len(records)
	r.mu.Unlock()
	r.reporter.Done(key, len(records), time.Since(started))
	return true
}

func (r *syncRun) list(resourceType, path string, filter url.Values) {
	r.dataset(sessionDataset(resourceType), func() ([]store.Record, error) {
		items, err := r.api.listAll(path, filter)
		if err != nil {
			return nil, err
		}
		return recordsFromItems(items)
	})
}

func (r *syncRun) feed(resourceType, path string, first url.Values) {
	r.dataset(sessionDataset(resourceType), func() ([]store.Record, error) {
		items, err := r.api.listFeed(path, first)
		if err != nil {
			return nil, err
		}
		return recordsFromItems(items)
	})
}

type rosterProfile struct {
	ID        int     `json:"id"`
	ReportsTo *string `json:"reports_to"`
}

func (p rosterProfile) managerID() (int, bool, error) {
	if p.ReportsTo == nil || *p.ReportsTo == "" {
		return 0, false, nil
	}
	id, err := profileIDFromURI(*p.ReportsTo)
	if err != nil {
		return 0, false, fmt.Errorf("user profile %d has an invalid reports_to: %w", p.ID, err)
	}
	return id, true, nil
}

func (r *syncRun) fetchRoster() ([]json.RawMessage, []int, error) {
	items, err := r.api.listAll("/api/v1/userprofile/", nil)
	if err != nil {
		return nil, nil, err
	}
	profiles := make(map[int]rosterProfile, len(items))
	add := func(item json.RawMessage) (rosterProfile, error) {
		var profile rosterProfile
		if err := json.Unmarshal(item, &profile); err != nil {
			return profile, fmt.Errorf("decoding user profile: %w", err)
		}
		if profile.ID <= 0 {
			return profile, errors.New("a user profile has no id")
		}
		profiles[profile.ID] = profile
		return profile, nil
	}
	for _, item := range items {
		if _, err := add(item); err != nil {
			return nil, nil, err
		}
	}
	ensure := func(id int) (rosterProfile, error) {
		if profile, ok := profiles[id]; ok {
			return profile, nil
		}
		item, err := r.api.getObject(profileURI(id))
		if err != nil {
			return rosterProfile{}, err
		}
		profile, err := add(item)
		if err != nil {
			return rosterProfile{}, err
		}
		if profile.ID != id {
			return rosterProfile{}, fmt.Errorf("user profile %d was requested but profile %d was returned", id, profile.ID)
		}
		items = append(items, item)
		return profile, nil
	}

	self, err := ensure(r.profileID)
	if err != nil {
		return nil, nil, fmt.Errorf("own user profile: %w", err)
	}
	managerID, hasManager, err := self.managerID()
	if err != nil {
		return nil, nil, err
	}
	if hasManager {
		if _, err := ensure(managerID); err != nil {
			return nil, nil, fmt.Errorf("manager user profile: %w", err)
		}
	}

	var reports []int
	for id, profile := range profiles {
		reportsTo, ok, err := profile.managerID()
		if err != nil {
			return nil, nil, err
		}
		if ok && reportsTo == r.profileID && id != r.profileID {
			reports = append(reports, id)
		}
	}
	sort.Ints(reports)
	return items, reports, nil
}

func (r *syncRun) syncUser(profileID int) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.dataset(userDataset(resourceUserObjectives, profileID), func() ([]store.Record, error) {
			return fetchUserObjectives(r.api, profileID)
		})
	}()
	go func() {
		defer wg.Done()
		var reviewRequestIDs []int
		reviewsSynced := r.dataset(userDataset(resourceUserSnapshots, profileID), func() ([]store.Record, error) {
			records, linked, err := fetchUserSnapshots(r.api, profileID)
			reviewRequestIDs = linked
			return records, err
		})
		if profileID == r.profileID {
			r.syncFeedback(reviewRequestIDs, reviewsSynced)
		}
	}()
	wg.Wait()
}

func (r *syncRun) syncFeedback(reviewRequestIDs []int, reviewsSynced bool) {
	key := sessionDataset(resourceFeedbackRequests)
	if !reviewsSynced {
		r.reporter.Start(key)
		r.fail(key, fmt.Errorf("not synced because %s failed", resourceUserSnapshots), 0)
		return
	}
	r.dataset(key, func() ([]store.Record, error) {
		return fetchFeedbackRequests(r.api, r.profileID, reviewRequestIDs)
	})
}

func (r *syncRun) syncRosterAndReports() {
	var reports []int
	ok := r.dataset(sessionDataset(resourceUserProfile), func() ([]store.Record, error) {
		items, reportIDs, err := r.fetchRoster()
		if err != nil {
			return nil, err
		}
		reports = reportIDs
		return recordsFromItems(items)
	})
	if !ok {
		return
	}
	var wg sync.WaitGroup
	for _, reportID := range reports {
		wg.Add(1)
		go func(reportID int) {
			defer wg.Done()
			r.syncUser(reportID)
		}(reportID)
	}
	wg.Wait()
}

func (r *syncRun) syncMeetings() {
	var completed []int
	ok := r.dataset(sessionDataset(resourceOneonones), func() ([]store.Record, error) {
		items, err := r.api.listAll("/api/v1/oneonones/", nil)
		if err != nil {
			return nil, err
		}
		completed = nil
		for _, item := range items {
			var meeting struct {
				ID     int  `json:"id"`
				Status *int `json:"status"`
			}
			if err := json.Unmarshal(item, &meeting); err != nil {
				return nil, fmt.Errorf("decoding 1:1 meeting: %w", err)
			}
			if meeting.ID <= 0 || meeting.Status == nil {
				return nil, fmt.Errorf("1:1 meeting %d has no id or no status", meeting.ID)
			}
			if *meeting.Status == oneononeStatusCompleted {
				completed = append(completed, meeting.ID)
			}
		}
		sort.Ints(completed)
		return recordsFromItems(items)
	})
	finalized := sessionDataset(resourceFinalizedOneonones)
	if !ok {
		r.reporter.Start(finalized)
		r.fail(finalized, fmt.Errorf("not synced because %s failed", resourceOneonones), 0)
		return
	}
	r.dataset(finalized, func() ([]store.Record, error) {
		return fetchFinalizedMeetings(r.api, completed)
	})
}

func (r *syncRun) execute() {
	self := strconv.Itoa(r.profileID)
	tasks := []func(){
		r.syncRosterAndReports,
		r.syncMeetings,
		func() { r.syncUser(r.profileID) },
		func() { r.list(resourceOneononeNotes, "/api/v1/oneononenotes/", nil) },
		func() {
			r.list(resourcePerformanceCycles, "/api/v1/performancecycles/", url.Values{"target": {self}})
		},
		func() {
			r.list(resourceRecognitionGiven, "/api/v1/recognitionbadges/", url.Values{"sender": {self}})
		},
		func() { r.list(resourceCheckins, "/api/v1/checkins/", url.Values{"creator": {self}}) },
		func() {
			r.feed(resourceRecognitionGot, "/api/v1/feeds/recognition/", url.Values{
				"recipient": {profileURI(r.profileID)},
				"limit":     {strconv.Itoa(feedPageSize)},
			})
		},
		func() {
			r.feed(resourceNotifications, "/api/v1/notifications/", url.Values{"limit": {strconv.Itoa(feedPageSize)}})
		},
	}
	var wg sync.WaitGroup
	for _, task := range tasks {
		wg.Add(1)
		go func(task func()) {
			defer wg.Done()
			task()
		}(task)
	}
	wg.Wait()
}

func runSync(ctx context.Context, db *store.Store, opts syncOptions) error {
	if opts.Reporter == nil {
		return errors.New("sync has no reporter")
	}
	started := time.Now()
	api, profileID, err := openSyncSession(ctx, opts)
	if err != nil {
		return err
	}
	if err := claimDatabase(db, profileID); err != nil {
		return err
	}

	run := &syncRun{api: api, db: db, reporter: opts.Reporter, profileID: profileID}
	run.execute()

	if len(run.failed) == 0 {
		purged, err := db.PurgeExcept(append(run.synced, sessionDataset(resourceSessionUser)))
		if err != nil {
			return fmt.Errorf("removing data that is not part of the sync: %w", err)
		}
		for _, key := range purged {
			opts.Reporter.Purged(key)
		}
		if len(purged) > 0 {
			if err := db.Compact(); err != nil {
				return err
			}
		}
	}
	opts.Reporter.Summary(run.records, len(run.synced), len(run.failed), time.Since(started))
	if len(run.failed) > 0 {
		names := make([]string, 0, len(run.failed))
		for _, key := range run.failed {
			names = append(names, key.String())
		}
		sort.Strings(names)
		return fmt.Errorf("%d of %d datasets failed to sync: %s; the earlier local data of these datasets was kept, run sync again", len(run.failed), len(run.synced)+len(run.failed), strings.Join(names, ", "))
	}
	return nil
}
