// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

const (
	syncMaxAttempts           = 4
	syncMaxConcurrentRequests = 8
	syncMaxFeedPages          = 10000
)

var syncRetryBaseDelay = time.Second

var syncListPageSizes = []int{1000, 611, 389, 100}

var errNotLoggedIn = errors.New("the 7Geese session is not logged in")

type apiStatusError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *apiStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s returned HTTP %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s returned HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

type syncAPI struct {
	ctx     context.Context
	baseURL string
	session string
	csrf    string
	client  *http.Client
	slots   chan struct{}

	rateMu      sync.Mutex
	minInterval time.Duration
	nextRequest time.Time
}

func newSyncAPI(ctx context.Context, cfg *config.Config, timeout time.Duration, requestsPerSecond float64) (*syncAPI, error) {
	if cfg.SevengeeseSession == "" {
		return nil, errors.New("not authenticated: run '7geese-cli auth login' first")
	}
	if cfg.BaseURL == "" {
		return nil, errors.New("config has no base_url")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("request timeout must be positive, got %s", timeout)
	}
	if requestsPerSecond < 0 {
		return nil, fmt.Errorf("rate limit must not be negative, got %v", requestsPerSecond)
	}
	api := &syncAPI{
		ctx:     ctx,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		session: cfg.SevengeeseSession,
		csrf:    cfg.SevengeeseCSRF,
		client:  &http.Client{Timeout: timeout},
		slots:   make(chan struct{}, syncMaxConcurrentRequests),
	}
	if requestsPerSecond > 0 {
		api.minInterval = time.Duration(float64(time.Second) / requestsPerSecond)
	}
	return api, nil
}

func (a *syncAPI) wait(delay time.Duration) error {
	if delay <= 0 {
		return a.ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-a.ctx.Done():
		return a.ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (a *syncAPI) waitForRateLimit() error {
	if a.minInterval == 0 {
		return nil
	}
	a.rateMu.Lock()
	now := time.Now()
	start := a.nextRequest
	if start.Before(now) {
		start = now
	}
	a.nextRequest = start.Add(a.minInterval)
	a.rateMu.Unlock()
	return a.wait(start.Sub(now))
}

func (a *syncAPI) send(method, path string, query url.Values, body []byte, referer string) ([]byte, error) {
	select {
	case a.slots <- struct{}{}:
	case <-a.ctx.Done():
		return nil, a.ctx.Err()
	}
	defer func() { <-a.slots }()

	target := a.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var lastErr error
	for attempt := 1; attempt <= syncMaxAttempts; attempt++ {
		if attempt > 1 {
			if err := a.wait(syncRetryBaseDelay << (attempt - 2)); err != nil {
				return nil, err
			}
		}
		if err := a.waitForRateLimit(); err != nil {
			return nil, err
		}

		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(a.ctx, method, target, reader)
		if err != nil {
			return nil, fmt.Errorf("building %s %s: %w", method, path, err)
		}
		cookie := "sgsession4=" + a.session
		if a.csrf != "" {
			cookie += "; sgcsrftoken4=" + a.csrf
		}
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "7geese-cli/"+version)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CSRFToken", a.csrf)
			req.Header.Set("Referer", a.baseURL+referer)
			req.Header.Set("X-HREF", a.baseURL+referer)
			req.Header.Set("Origin", a.baseURL)
		}

		resp, err := a.client.Do(req)
		if err != nil {
			if a.ctx.Err() != nil {
				return nil, a.ctx.Err()
			}
			lastErr = fmt.Errorf("%s %s: %w", method, path, err)
			continue
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("reading response of %s %s: %w", method, path, readErr)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return data, nil
		}
		statusErr := &apiStatusError{Method: method, Path: path, Status: resp.StatusCode, Body: excerpt(data, 300)}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = statusErr
			continue
		}
		return nil, statusErr
	}
	return nil, fmt.Errorf("%d attempts failed: %w", syncMaxAttempts, lastErr)
}

func excerpt(data []byte, limit int) string {
	text := strings.TrimSpace(string(data))
	if len(text) > limit {
		return text[:limit] + "..."
	}
	return text
}

func (a *syncAPI) getJSON(path string, query url.Values, out any) error {
	data, err := a.send(http.MethodGet, path, query, nil, "")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding response of GET %s: %w (body: %s)", path, err, excerpt(data, 200))
	}
	return nil
}

func (a *syncAPI) graphQL(operation, referer, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{
		"operationName": operation,
		"variables":     variables,
		"query":         query,
	})
	if err != nil {
		return fmt.Errorf("encoding GraphQL request %s: %w", operation, err)
	}
	data, err := a.send(http.MethodPost, "/graphql", url.Values{"opname": {operation}}, body, referer)
	if err != nil {
		return err
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decoding GraphQL response of %s: %w (body: %s)", operation, err, excerpt(data, 200))
	}
	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			messages = append(messages, e.Message)
		}
		return fmt.Errorf("GraphQL %s returned errors: %s", operation, strings.Join(messages, "; "))
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return fmt.Errorf("GraphQL %s returned no data", operation)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("decoding GraphQL data of %s: %w", operation, err)
	}
	return nil
}

func (a *syncAPI) currentUserID() (int, error) {
	var status struct {
		LoggedIn bool `json:"loggedIn"`
		UserID   int  `json:"userId"`
	}
	if err := a.getJSON("/status/", nil, &status); err != nil {
		return 0, err
	}
	if !status.LoggedIn || status.UserID == 0 {
		return 0, errNotLoggedIn
	}
	return status.UserID, nil
}

func isSessionRejected(err error) bool {
	if errors.Is(err, errNotLoggedIn) {
		return true
	}
	var statusErr *apiStatusError
	return errors.As(err, &statusErr) && statusErr.Status == http.StatusUnauthorized
}

func (a *syncAPI) listPass(path string, filter url.Values, pageSize int, add func(json.RawMessage) error) (int, error) {
	fetched := 0
	for {
		query := url.Values{}
		for name, values := range filter {
			query[name] = values
		}
		query.Set("limit", strconv.Itoa(pageSize))
		query.Set("offset", strconv.Itoa(fetched))

		var page struct {
			Meta *struct {
				TotalCount *int    `json:"total_count"`
				Next       *string `json:"next"`
			} `json:"meta"`
			Objects *[]json.RawMessage `json:"objects"`
		}
		if err := a.getJSON(path, query, &page); err != nil {
			return 0, err
		}
		if page.Meta == nil || page.Meta.TotalCount == nil || page.Objects == nil {
			return 0, fmt.Errorf("GET %s did not return a list with meta.total_count and objects", path)
		}
		for _, item := range *page.Objects {
			if err := add(item); err != nil {
				return 0, fmt.Errorf("GET %s: %w", path, err)
			}
		}
		fetched += len(*page.Objects)
		if fetched > *page.Meta.TotalCount {
			return 0, fmt.Errorf("GET %s returned %d records but reports a total of %d; the list does not obey the offset or it changed during the sync, run it again", path, fetched, *page.Meta.TotalCount)
		}
		if page.Meta.Next == nil || *page.Meta.Next == "" {
			return *page.Meta.TotalCount, nil
		}
		if len(*page.Objects) == 0 {
			return 0, fmt.Errorf("GET %s returned an empty page before the end of the list (offset %d)", path, fetched)
		}
	}
}

func (a *syncAPI) listAll(path string, filter url.Values) ([]json.RawMessage, error) {
	var all []json.RawMessage
	seen := map[string]bool{}
	add := func(item json.RawMessage) error {
		id, err := recordID(item)
		if err != nil {
			return err
		}
		if !seen[id] {
			seen[id] = true
			all = append(all, item)
		}
		return nil
	}
	total := 0
	for _, pageSize := range syncListPageSizes {
		var err error
		total, err = a.listPass(path, filter, pageSize, add)
		if err != nil {
			return nil, err
		}
		if len(all) == total {
			return all, nil
		}
		if len(all) > total {
			break
		}
	}
	return nil, fmt.Errorf("GET %s returned %d different records but reports a total of %d (page sizes %v); the list is not stable or it changed during the sync, run it again", path, len(all), total, syncListPageSizes)
}

func (a *syncAPI) getObject(path string) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := a.getJSON(path, nil, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || raw[0] != '{' {
		return nil, fmt.Errorf("GET %s did not return an object", path)
	}
	return raw, nil
}

func (a *syncAPI) listFeed(path string, first url.Values) ([]json.RawMessage, error) {
	var all []json.RawMessage
	seen := map[string]bool{}
	query := first
	for pageNumber := 1; pageNumber <= syncMaxFeedPages; pageNumber++ {
		var page struct {
			Meta *struct {
				Next *string `json:"next"`
			} `json:"meta"`
			Objects *[]json.RawMessage `json:"objects"`
		}
		if err := a.getJSON(path, query, &page); err != nil {
			return nil, err
		}
		if page.Meta == nil || page.Objects == nil {
			return nil, fmt.Errorf("GET %s did not return a feed with meta and objects", path)
		}
		terminal := page.Meta.Next == nil || *page.Meta.Next == ""
		var nextQuery url.Values
		if !terminal {
			next, err := url.Parse(*page.Meta.Next)
			if err != nil {
				return nil, fmt.Errorf("GET %s returned an invalid next link %q: %w", path, *page.Meta.Next, err)
			}
			if next.Path != path {
				return nil, fmt.Errorf("GET %s returned a next link to a different path %q", path, next.Path)
			}
			nextQuery = next.Query()
			terminal = nextQuery.Get("offset") == "None"
		}
		if len(*page.Objects) == 0 && !terminal {
			return nil, fmt.Errorf("GET %s page %d is empty but has a next link; the end of the feed is not clear", path, pageNumber)
		}
		added := 0
		for _, item := range *page.Objects {
			id, err := recordID(item)
			if err != nil {
				return nil, fmt.Errorf("GET %s: %w", path, err)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			all = append(all, item)
			added++
		}
		if terminal {
			return all, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("GET %s page %d has no new records; the feed does not advance", path, pageNumber)
		}
		if nextQuery.Encode() == query.Encode() {
			return nil, fmt.Errorf("GET %s page %d returned a next link that does not advance", path, pageNumber)
		}
		query = nextQuery
	}
	return nil, fmt.Errorf("GET %s did not end after %d pages", path, syncMaxFeedPages)
}

func recordID(item json.RawMessage) (string, error) {
	var obj struct {
		ID json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal(item, &obj); err != nil {
		return "", fmt.Errorf("record is not a JSON object: %w", err)
	}
	return canonicalID(obj.ID)
}

func canonicalID(raw json.RawMessage) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return "", errors.New("record has no id")
	}
	if text[0] == '"' {
		var id string
		if err := json.Unmarshal(raw, &id); err != nil {
			return "", fmt.Errorf("record id %s is not a valid string: %w", text, err)
		}
		if id == "" {
			return "", errors.New("record has an empty id")
		}
		return id, nil
	}
	if _, err := strconv.ParseInt(text, 10, 64); err != nil {
		return "", fmt.Errorf("record id %s is not an integer or a string", text)
	}
	return text, nil
}

func recordsFromItems(items []json.RawMessage) ([]store.Record, error) {
	records := make([]store.Record, 0, len(items))
	for _, item := range items {
		id, err := recordID(item)
		if err != nil {
			return nil, err
		}
		records = append(records, store.Record{ID: id, Data: item})
	}
	return records, nil
}

func fetchEach[T any](ids []int, fetch func(int) (T, error)) ([]T, error) {
	results := make([]T, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			results[i], errs[i] = fetch(id)
		}(i, id)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}
