// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"7geese-cli/internal/store"
)

const (
	objectivePageSize        = 50
	objectiveCheckinPageSize = 100
	objectiveChildPageSize   = 200
)

var objectiveFilters = []string{"ownerOrStakeholder", "follower"}

type gqlPageInfo struct {
	EndCursor   string `json:"endCursor"`
	HasNextPage bool   `json:"hasNextPage"`
}

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
	PK       int              `json:"pk"`
	Message  string           `json:"message"`
	Created  string           `json:"created"`
	Metadata json.RawMessage  `json:"metadata,omitempty"`
	User     gqlObjectiveUser `json:"user"`
}

type gqlObjectiveCheckinEdge struct {
	Node gqlObjectiveCheckin `json:"node"`
}

type gqlObjectiveCheckins struct {
	TotalCount int                       `json:"totalCount"`
	PageInfo   *gqlPageInfo              `json:"pageInfo,omitempty"`
	Edges      []gqlObjectiveCheckinEdge `json:"edges"`
}

type gqlObjectiveNode struct {
	PK                         int     `json:"pk"`
	Name                       string  `json:"name"`
	Description                string  `json:"description"`
	Closed                     bool    `json:"closed"`
	Draft                      bool    `json:"draft"`
	Private                    bool    `json:"private"`
	Progress                   float64 `json:"progress"`
	DueDatetime                *string `json:"dueDatetime"`
	StartDate                  *string `json:"startDate"`
	CompletedDatetime          *string `json:"completedDatetime"`
	ObjectiveType              int     `json:"objectiveType"`
	CurrentUserParticipantType int     `json:"currentUserParticipantType"`
	Owners                     struct {
		PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
		Edges    []struct {
			Node gqlObjectiveUser `json:"node"`
		} `json:"edges"`
	} `json:"owners"`
	KeyResults struct {
		PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
		Edges    []struct {
			Node gqlObjectiveKR `json:"node"`
		} `json:"edges"`
	} `json:"keyResults"`
	LastCheckin *gqlObjectiveCheckin `json:"lastCheckin"`
	Checkins    gqlObjectiveCheckins `json:"checkins"`
}

type gqlObjectivePage struct {
	TotalCount int         `json:"totalCount"`
	PageInfo   gqlPageInfo `json:"pageInfo"`
	Edges      []struct {
		Node gqlObjectiveNode `json:"node"`
	} `json:"edges"`
}

const objectiveCheckinFields = `totalCount
          pageInfo { endCursor hasNextPage }
          edges { node { pk message created metadata user { pk fullName } } }`

func objectivesQuery(filter string) string {
	return fmt.Sprintf(`
query getUserContextObjectives($userId: Int!, $first: Int!, $after: String, $children: Int!, $checkins: Int!) {
  objectives(first: $first, after: $after, %s: $userId, orderBy: "closed,-due_datetime,pk") {
    totalCount
    pageInfo { endCursor hasNextPage }
    edges {
      node {
        pk name description closed draft private progress
        dueDatetime startDate completedDatetime objectiveType currentUserParticipantType
        owners(first: $children) {
          pageInfo { endCursor hasNextPage }
          edges { node { pk fullName position } }
        }
        keyResults(first: $children) {
          pageInfo { endCursor hasNextPage }
          edges { node { id: pk name description progress currentValue targetValue startingValue measurementType } }
        }
        checkins(first: $checkins) {
          %s
        }
        lastCheckin { pk message created user { pk fullName } }
      }
    }
  }
}`, filter, objectiveCheckinFields)
}

var objectiveCheckinsQuery = fmt.Sprintf(`
query getObjectiveCheckins($pk: Int!, $first: Int!, $after: String) {
  objective(pk: $pk) {
    pk
    checkins(first: $first, after: $after) {
          %s
    }
  }
}`, objectiveCheckinFields)

func fetchObjectivesPage(api *syncAPI, filter string, profileID int, after string) (*gqlObjectivePage, error) {
	var data struct {
		Objectives *gqlObjectivePage `json:"objectives"`
	}
	err := api.graphQL("getUserContextObjectives", "/objectives/", objectivesQuery(filter), map[string]any{
		"userId":   profileID,
		"first":    objectivePageSize,
		"after":    after,
		"children": objectiveChildPageSize,
		"checkins": objectiveCheckinPageSize,
	}, &data)
	if err != nil {
		return nil, err
	}
	if data.Objectives == nil {
		return nil, fmt.Errorf("GraphQL getUserContextObjectives returned no objectives list for user %d", profileID)
	}
	return data.Objectives, nil
}

func fetchObjectiveCheckinsPage(api *syncAPI, objectiveID int, after string) (*gqlObjectiveCheckins, error) {
	var data struct {
		Objective *struct {
			Checkins gqlObjectiveCheckins `json:"checkins"`
		} `json:"objective"`
	}
	err := api.graphQL("getObjectiveCheckins", "/objectives/", objectiveCheckinsQuery, map[string]any{
		"pk":    objectiveID,
		"first": objectiveCheckinPageSize,
		"after": after,
	}, &data)
	if err != nil {
		return nil, err
	}
	if data.Objective == nil {
		return nil, fmt.Errorf("objective %d is not visible; its check-in history cannot be read", objectiveID)
	}
	return &data.Objective.Checkins, nil
}

func completeObjective(api *syncAPI, node *gqlObjectiveNode) error {
	if node.PK == 0 {
		return fmt.Errorf("objective %q has no pk", node.Name)
	}
	if node.Owners.PageInfo == nil || node.KeyResults.PageInfo == nil || node.Checkins.PageInfo == nil {
		return fmt.Errorf("objective %d has no page information for its owners, key results or check-ins", node.PK)
	}
	if node.Owners.PageInfo.HasNextPage {
		return fmt.Errorf("objective %d has more than %d owners", node.PK, objectiveChildPageSize)
	}
	if node.KeyResults.PageInfo.HasNextPage {
		return fmt.Errorf("objective %d has more than %d key results", node.PK, objectiveChildPageSize)
	}
	node.Owners.PageInfo = nil
	node.KeyResults.PageInfo = nil

	pageInfo := *node.Checkins.PageInfo
	for pageInfo.HasNextPage {
		if pageInfo.EndCursor == "" {
			return fmt.Errorf("objective %d check-in history has a next page but no cursor", node.PK)
		}
		cursor := pageInfo.EndCursor
		page, err := fetchObjectiveCheckinsPage(api, node.PK, cursor)
		if err != nil {
			return fmt.Errorf("check-in history of objective %d: %w", node.PK, err)
		}
		if page.PageInfo == nil {
			return fmt.Errorf("objective %d check-in page has no page information", node.PK)
		}
		if len(page.Edges) == 0 {
			return fmt.Errorf("objective %d check-in history returned an empty page", node.PK)
		}
		node.Checkins.Edges = append(node.Checkins.Edges, page.Edges...)
		node.Checkins.TotalCount = page.TotalCount
		if len(node.Checkins.Edges) > page.TotalCount {
			return fmt.Errorf("objective %d returned more than its %d check-ins; the check-in history does not advance", node.PK, page.TotalCount)
		}
		if page.PageInfo.HasNextPage && page.PageInfo.EndCursor == cursor {
			return fmt.Errorf("objective %d check-in history does not advance after cursor %q", node.PK, cursor)
		}
		pageInfo = *page.PageInfo
	}
	node.Checkins.PageInfo = nil

	seen := make(map[int]bool, len(node.Checkins.Edges))
	for _, edge := range node.Checkins.Edges {
		if edge.Node.PK == 0 {
			return fmt.Errorf("objective %d has a check-in with no pk", node.PK)
		}
		if seen[edge.Node.PK] {
			return fmt.Errorf("objective %d returned check-in %d two times", node.PK, edge.Node.PK)
		}
		seen[edge.Node.PK] = true
	}
	if len(node.Checkins.Edges) != node.Checkins.TotalCount {
		return fmt.Errorf("objective %d returned %d of %d check-ins", node.PK, len(node.Checkins.Edges), node.Checkins.TotalCount)
	}
	return nil
}

func fetchUserObjectives(api *syncAPI, profileID int) ([]store.Record, error) {
	all := map[int]gqlObjectiveNode{}
	for _, filter := range objectiveFilters {
		after := ""
		fetched := map[int]bool{}
		for {
			page, err := fetchObjectivesPage(api, filter, profileID, after)
			if err != nil {
				return nil, fmt.Errorf("objectives (%s): %w", filter, err)
			}
			for i := range page.Edges {
				node := page.Edges[i].Node
				if err := completeObjective(api, &node); err != nil {
					return nil, fmt.Errorf("objectives (%s): %w", filter, err)
				}
				fetched[node.PK] = true
				all[node.PK] = node
			}
			if !page.PageInfo.HasNextPage {
				if len(fetched) != page.TotalCount {
					return nil, fmt.Errorf("objectives (%s): received %d of %d objectives; the list changed during the sync, run it again", filter, len(fetched), page.TotalCount)
				}
				break
			}
			if len(page.Edges) == 0 || page.PageInfo.EndCursor == "" || page.PageInfo.EndCursor == after {
				return nil, fmt.Errorf("objectives (%s): the list does not advance after cursor %q", filter, after)
			}
			after = page.PageInfo.EndCursor
		}
	}

	ids := make([]int, 0, len(all))
	for id := range all {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	records := make([]store.Record, 0, len(ids))
	for _, id := range ids {
		data, err := json.Marshal(all[id])
		if err != nil {
			return nil, fmt.Errorf("encoding objective %d: %w", id, err)
		}
		records = append(records, store.Record{ID: strconv.Itoa(id), Data: data})
	}
	return records, nil
}
