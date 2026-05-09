// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

var syncHTTPClient = &http.Client{Timeout: 30 * time.Second}

// fetchCurrentUserID calls /status/ and returns the authenticated user's
// profile ID. This is the canonical whoami for session-cookie auth — one
// lightweight GET, no variables required.
func fetchCurrentUserID(flags *rootFlags) (int, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return 0, fmt.Errorf("no auth configured")
	}
	req, err := http.NewRequest("GET", cfg.BaseURL+"/status/", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
	resp, err := syncHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	var status struct {
		LoggedIn bool `json:"loggedIn"`
		UserID   int  `json:"userId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return 0, err
	}
	if !status.LoggedIn || status.UserID == 0 {
		return 0, fmt.Errorf("session not logged in")
	}
	return status.UserID, nil
}

// syncUserRecognition fetches all recognition badges sent or received by the
// current user via two filtered REST calls and stores them in the local store.
// This replaces the unfiltered company-wide recognitionbadges sync.
func syncUserRecognition(flags *rootFlags, db *store.Store, profileID int) (int, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return 0, fmt.Errorf("no auth configured")
	}

	type restPage struct {
		Objects []json.RawMessage `json:"objects"`
		Meta    struct {
			TotalCount int `json:"total_count"`
			Next       string `json:"next"`
		} `json:"meta"`
	}

	profileIDStr := strconv.Itoa(profileID)
	fetchPage := func(filter string, offset int) (*restPage, error) {
		q := url.Values{}
		q.Set(filter, profileIDStr)
		q.Set("limit", "100")
		q.Set("offset", strconv.Itoa(offset))
		u := cfg.BaseURL + "/api/v1/recognitionbadges/?" + q.Encode()
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var page restPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			return nil, err
		}
		return &page, nil
	}

	// Sent badges: recognitionbadges?sender=<id> (numeric ID, not URI).
	// recipient=<id> returns 25k+ company-wide records so we skip it here.
	synced := 0
	offset := 0
	for {
		page, err := fetchPage("sender", offset)
		if err != nil {
			return synced, fmt.Errorf("fetching recognitionbadges: %w", err)
		}
		for _, item := range page.Objects {
			var obj struct {
				ID int `json:"id"`
			}
			if err := json.Unmarshal(item, &obj); err != nil || obj.ID == 0 {
				continue
			}
			_ = db.Upsert("recognitionbadges", strconv.Itoa(obj.ID), item)
			synced++
		}
		if len(page.Objects) < 100 {
			break
		}
		offset += 100
	}

	// Received badges: feeds/recognition?recipient=<profileURI> uses timestamp cursors.
	profileURI := fmt.Sprintf("/api/v1/userprofile/%d/", profileID)
	rSynced, rErr := syncReceivedRecognition(flags, cfg, db, profileURI)
	synced += rSynced
	if rErr != nil {
		return synced, rErr
	}

	return synced, nil
}

func syncReceivedRecognition(flags *rootFlags, cfg *config.Config, db *store.Store, profileURI string) (int, error) {
	type feedMeta struct {
		Limit  int    `json:"limit"`
		Offset string `json:"offset"`
	}
	type feedPage struct {
		Objects []json.RawMessage `json:"objects"`
		Meta    feedMeta          `json:"meta"`
	}

	synced := 0
	offsetParam := ""
	const limit = 100
	for {
		q := url.Values{}
		q.Set("recipient", profileURI)
		q.Set("limit", strconv.Itoa(limit))
		if offsetParam != "" && offsetParam != "None" {
			q.Set("offset", offsetParam)
		}
		u := cfg.BaseURL + "/api/v1/feeds/recognition/?" + q.Encode()
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return synced, err
		}
		req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			return synced, fmt.Errorf("fetching feeds/recognition: %w", err)
		}
		var page feedPage
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		if err != nil {
			return synced, fmt.Errorf("decoding feeds/recognition: %w", err)
		}
		for _, item := range page.Objects {
			var obj struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(item, &obj); err != nil || obj.ID == "" {
				continue
			}
			_ = db.Upsert("recognition_received", obj.ID, item)
			synced++
		}
		if len(page.Objects) < limit || page.Meta.Offset == "None" || page.Meta.Offset == "" || page.Meta.Offset == offsetParam {
			break
		}
		offsetParam = page.Meta.Offset
	}
	return synced, nil
}

// --- GraphQL types for getUserContextObjectives ---

type gqlObjectiveUser struct {
	PK       int    `json:"pk"`
	FullName string `json:"fullName"`
	Position string `json:"position"`
}

type gqlObjectiveKR struct {
	PK              int     `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	Progress        float64 `json:"progress"`
	CurrentValue    float64 `json:"currentValue"`
	TargetValue     float64 `json:"targetValue"`
	StartingValue   float64 `json:"startingValue"`
	MeasurementType int     `json:"measurementType"`
}

type gqlObjectiveCheckin struct {
	PK      int              `json:"pk"`
	Message string           `json:"message"`
	Created string           `json:"created"`
	User    gqlObjectiveUser `json:"user"`
}

type gqlObjectiveNode struct {
	PK                       int     `json:"pk"`
	Name                     string  `json:"name"`
	Description              string  `json:"description"`
	Closed                   bool    `json:"closed"`
	Draft                    bool    `json:"draft"`
	Private                  bool    `json:"private"`
	Progress                 float64 `json:"progress"`
	DueDatetime              string  `json:"dueDatetime"`
	StartDate                string  `json:"startDate"`
	CompletedDatetime        string  `json:"completedDatetime"`
	ObjectiveType            int     `json:"objectiveType"`
	CurrentUserParticipantType int   `json:"currentUserParticipantType"`
	Owners struct {
		Edges []struct {
			Node gqlObjectiveUser `json:"node"`
		} `json:"edges"`
	} `json:"owners"`
	KeyResults struct {
		Edges []struct {
			Node gqlObjectiveKR `json:"node"`
		} `json:"edges"`
	} `json:"keyResults"`
	LastCheckin *gqlObjectiveCheckin `json:"lastCheckin"`
	Checkins    struct {
		TotalCount int `json:"totalCount"`
	} `json:"checkins"`
}

type gqlObjectivePage struct {
	TotalCount int `json:"totalCount"`
	PageInfo   struct {
		EndCursor   string `json:"endCursor"`
		HasNextPage bool   `json:"hasNextPage"`
	} `json:"pageInfo"`
	Edges []struct {
		Node gqlObjectiveNode `json:"node"`
	} `json:"edges"`
}

func fetchContextObjectivesPage(flags *rootFlags, cfg *config.Config, filter string, profileID int, after string) (*gqlObjectivePage, error) {
	variables := map[string]any{
		"userId": profileID,
		"first":  100,
		"after":  after,
	}

	// Build the query dynamically with the correct filter arg
	query := fmt.Sprintf(`
query getUserContextObjectives($userId: Float!, $first: Int!, $after: String) {
  objectives(first: $first, after: $after, %s: $userId, orderBy: "closed,-due_datetime") {
    totalCount
    pageInfo { endCursor hasNextPage __typename }
    edges {
      node {
        pk name description closed draft private progress
        dueDatetime startDate completedDatetime objectiveType currentUserParticipantType
        owners(first: 50) {
          edges { node { pk fullName position __typename } __typename }
          __typename
        }
        keyResults(first: 50) {
          edges {
            node {
              id: pk name description progress currentValue targetValue startingValue measurementType
              __typename
            }
            __typename
          }
          __typename
        }
        checkins { totalCount __typename }
        lastCheckin {
          pk message created
          user { pk fullName __typename }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}`, filter)

	body, _ := json.Marshal(map[string]any{
		"operationName": "getUserContextObjectives",
		"variables":     variables,
		"query":         query,
	})

	req, err := http.NewRequest("POST", cfg.BaseURL+"/graphql?opname=getUserContextObjectives", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	referer := cfg.BaseURL + "/objectives/"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
	req.Header.Set("X-CSRFToken", cfg.SevengeeseCSRF)
	req.Header.Set("Referer", referer)
	req.Header.Set("X-HREF", referer)
	req.Header.Set("Origin", cfg.BaseURL)

	resp, err := syncHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Data struct {
			Objectives gqlObjectivePage `json:"objectives"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return &envelope.Data.Objectives, nil
}

// syncCurrentUserProfile fetches the current user's profile and their manager's
// profile via direct GETs — no company-wide userprofile list needed.
func syncCurrentUserProfile(flags *rootFlags, db *store.Store, profileID int) error {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return fmt.Errorf("no auth configured")
	}

	fetchProfile := func(id int) (json.RawMessage, error) {
		u := cfg.BaseURL + fmt.Sprintf("/api/v1/userprofile/%d/", id)
		req, err := http.NewRequest("GET", u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var raw json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
			return nil, err
		}
		return raw, nil
	}

	// Fetch our own profile
	ownRaw, err := fetchProfile(profileID)
	if err != nil {
		return fmt.Errorf("fetching own profile: %w", err)
	}
	if err := db.Upsert("userprofile", strconv.Itoa(profileID), ownRaw); err != nil {
		return err
	}

	// Fetch manager's profile so name lookups work
	var profile struct {
		ReportsTo string `json:"reports_to"`
	}
	if json.Unmarshal(ownRaw, &profile) == nil && profile.ReportsTo != "" {
		// Extract ID from URI like /api/v1/userprofile/1940957/
		var managerID int
		fmt.Sscanf(profile.ReportsTo, "/api/v1/userprofile/%d/", &managerID)
		if managerID > 0 {
			managerRaw, err := fetchProfile(managerID)
			if err == nil {
				_ = db.Upsert("userprofile", strconv.Itoa(managerID), managerRaw)
			}
		}
	}
	return nil
}

// syncUserObjectives fetches all objectives where profileID is an owner,
// stakeholder, or follower via two GraphQL calls (ownerOrStakeholder + follower),
// deduplicates by pk, and stores each one as "user_objectives" in the local store.
func syncUserObjectives(flags *rootFlags, db *store.Store, profileID int, force bool) (int, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return 0, fmt.Errorf("no auth configured")
	}

	// Collect all unique objectives keyed by pk across both filter calls.
	all := map[int]gqlObjectiveNode{}

	for _, filter := range []string{"ownerOrStakeholder", "follower"} {
		after := ""
		for {
			page, err := fetchContextObjectivesPage(flags, cfg, filter, profileID, after)
			if err != nil {
				return 0, fmt.Errorf("fetching objectives (%s): %w", filter, err)
			}
			for _, edge := range page.Edges {
				all[edge.Node.PK] = edge.Node
			}
			if !page.PageInfo.HasNextPage {
				break
			}
			after = page.PageInfo.EndCursor
		}
	}

	synced := 0
	for _, node := range all {
		id := fmt.Sprintf("%d", node.PK)
		if !force {
			existing, _ := db.Get("user_objectives", id)
			if existing != nil {
				continue
			}
		}
		data, err := json.Marshal(node)
		if err != nil {
			continue
		}
		if err := db.Upsert("user_objectives", id, json.RawMessage(data)); err != nil {
			continue
		}
		synced++
	}
	return synced, nil
}
