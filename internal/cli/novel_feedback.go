// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"7geese-cli/internal/store"
)

type gqlNamedProfile struct {
	PK       int    `json:"pk"`
	FullName string `json:"fullName"`
}

type gqlFeedbackQuestion struct {
	TypeName    string `json:"__typename"`
	PK          int    `json:"pk"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Ordinal     *int   `json:"ordinal"`
	Start       *int   `json:"start,omitempty"`
	StartText   string `json:"startText,omitempty"`
	End         *int   `json:"end,omitempty"`
	EndText     string `json:"endText,omitempty"`
}

type gqlFeedbackComments struct {
	PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
	Edges    []struct {
		Node struct {
			PK      int              `json:"pk"`
			Created string           `json:"created"`
			Comment string           `json:"comment"`
			Creator *gqlNamedProfile `json:"creator"`
		} `json:"node"`
	} `json:"edges"`
}

type gqlFeedbackChoices struct {
	PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
	Edges    []struct {
		Node struct {
			Option struct {
				Title string `json:"title"`
			} `json:"option"`
		} `json:"node"`
	} `json:"edges"`
}

type gqlFeedbackAnswer struct {
	TypeName string               `json:"__typename"`
	PK       int                  `json:"pk"`
	Text     *string              `json:"text,omitempty"`
	Number   *int                 `json:"number,omitempty"`
	Choices  *gqlFeedbackChoices  `json:"choices,omitempty"`
	Question *gqlFeedbackQuestion `json:"question"`
	Comments gqlFeedbackComments  `json:"comments"`
}

type gqlFeedback struct {
	PK                 int             `json:"pk"`
	State              json.RawMessage `json:"state"`
	Created            string          `json:"created"`
	SubmittedTimestamp *string         `json:"submittedTimestamp"`
	Responder          *struct {
		Profile *gqlNamedProfile `json:"profile"`
	} `json:"responder"`
	Answers struct {
		PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
		Edges    []struct {
			Node gqlFeedbackAnswer `json:"node"`
		} `json:"edges"`
	} `json:"answers"`
	Comments gqlFeedbackComments `json:"comments"`
}

type gqlFeedbackRequest struct {
	PK                        int              `json:"pk"`
	Title                     string           `json:"title"`
	Description               string           `json:"description"`
	Type                      json.RawMessage  `json:"type"`
	State                     json.RawMessage  `json:"state"`
	StatusByRecipient         string           `json:"statusByRecipient"`
	Created                   string           `json:"created"`
	SentDate                  *string          `json:"sentDate"`
	DueDatetime               *string          `json:"dueDatetime"`
	IsArchived                *bool            `json:"isArchived"`
	RespondentsNamesVisibleTo json.RawMessage  `json:"respondentsNamesVisibleTo"`
	NumResponsesSubmitted     *int             `json:"numResponsesSubmitted"`
	CanViewResponses          *bool            `json:"canViewResponses"`
	CanViewAllResponses       *bool            `json:"canViewAllResponses"`
	Creator                   *gqlNamedProfile `json:"creator"`
	Recipient                 *gqlNamedProfile `json:"recipient"`
	Responders                struct {
		PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
		Edges    []struct {
			Node struct {
				PK      int              `json:"pk"`
				State   json.RawMessage  `json:"state"`
				Profile *gqlNamedProfile `json:"profile"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"responders"`
	QuestionSet *struct {
		Questions struct {
			PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
			Edges    []struct {
				Node gqlFeedbackQuestion `json:"node"`
			} `json:"edges"`
		} `json:"questions"`
	} `json:"questionSet"`
	Feedbacks struct {
		PageInfo *gqlPageInfo `json:"pageInfo,omitempty"`
		Edges    []struct {
			Node gqlFeedback `json:"node"`
		} `json:"edges"`
	} `json:"feedbacks"`
	CuratedReport *gqlCuratedReport `json:"curatedReport"`
}

const listFeedbackRequestsQuery = `
query listFeedbackRequests($profileId: Int!, $first: Int!, $offset: Int!) {
  feedbackRequests(
    recipients: [$profileId]
    includeArchived: true
    orderBy: "created"
    first: $first
    offset: $offset
  ) {
    totalCount
    edges {
      node {
        pk
        recipient {
          pk
        }
      }
    }
  }
}
`

const getFeedbackRequestQuery = `
fragment feedbackProfile on UserProfileNode {
  pk
  fullName
}

fragment feedbackQuestion on QuestionUnionNode {
  __typename
  ... on TextQuestionNode {
    pk
    type
    title
    description
    ordinal
  }
  ... on RangeQuestionNode {
    pk
    type
    title
    description
    ordinal
    start
    startText
    end
    endText
  }
  ... on NpsQuestionNode {
    pk
    type
    title
    description
    ordinal
  }
  ... on MultiOptionQuestionNode {
    pk
    type
    title
    description
    ordinal
  }
  ... on LikertQuestionNode {
    pk
    type
    title
    description
    ordinal
  }
  ... on SectionHeaderNode {
    pk
    type
    title
    description
    ordinal
  }
}

fragment feedbackAnswerComments on AnswerCommentNodeConnection {
  pageInfo {
    hasNextPage
  }
  edges {
    node {
      pk
      created
      comment
      creator {
        ...feedbackProfile
      }
    }
  }
}

fragment feedbackAnswer on AnswerUnionNode {
  __typename
  ... on TextAnswerNode {
    pk
    text: answer
    question {
      ...feedbackQuestion
    }
    comments(first: 200) {
      ...feedbackAnswerComments
    }
  }
  ... on RangeAnswerNode {
    pk
    number: answer
    question {
      ...feedbackQuestion
    }
    comments(first: 200) {
      ...feedbackAnswerComments
    }
  }
  ... on NpsAnswerNode {
    pk
    number: answer
    question {
      ...feedbackQuestion
    }
    comments(first: 200) {
      ...feedbackAnswerComments
    }
  }
  ... on LikertAnswerNode {
    pk
    number: answer
    question {
      ...feedbackQuestion
    }
    comments(first: 200) {
      ...feedbackAnswerComments
    }
  }
  ... on MultiOptionAnswerNode {
    pk
    choices(first: 200) {
      pageInfo {
        hasNextPage
      }
      edges {
        node {
          option {
            title
          }
        }
      }
    }
    question {
      ...feedbackQuestion
    }
    comments(first: 200) {
      ...feedbackAnswerComments
    }
  }
}

query getFeedbackRequest($pk: Int!) {
  feedbackRequest(pk: $pk) {
    pk
    title
    description
    type
    state
    statusByRecipient
    created
    sentDate
    dueDatetime
    isArchived
    respondentsNamesVisibleTo
    numResponsesSubmitted
    canViewResponses
    canViewAllResponses
    creator {
      ...feedbackProfile
    }
    recipient {
      ...feedbackProfile
    }
    responders(first: 200) {
      pageInfo {
        hasNextPage
      }
      edges {
        node {
          pk
          state
          profile {
            ...feedbackProfile
          }
        }
      }
    }
    questionSet {
      questions(first: 200) {
        pageInfo {
          hasNextPage
        }
        edges {
          node {
            ...feedbackQuestion
          }
        }
      }
    }
    feedbacks(first: 200) {
      pageInfo {
        hasNextPage
      }
      edges {
        node {
          pk
          state
          created
          submittedTimestamp
          responder {
            profile {
              ...feedbackProfile
            }
          }
          answers(first: 200) {
            pageInfo {
              hasNextPage
            }
            edges {
              node {
                ...feedbackAnswer
              }
            }
          }
          comments(first: 200) {
            pageInfo {
              hasNextPage
            }
            edges {
              node {
                pk
                created
                comment
                creator {
                  ...feedbackProfile
                }
              }
            }
          }
        }
      }
    }
    curatedReport {
      publishedDatetime
      sortedCuratedQuestions(first: 80) {
        pageInfo {
          hasNextPage
        }
        edges {
          node {
            comment
            question {
              ...feedbackQuestion
            }
            answers(first: 80) {
              pageInfo {
                hasNextPage
              }
              edges {
                node {
                  ... on TextAnswerNode {
                    answer
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
`

const feedbackRequestListPageSize = 50

func fetchFeedbackRequestIDs(api *syncAPI, profileID int) ([]int, error) {
	seen := map[int]bool{}
	received := 0
	for {
		var data struct {
			FeedbackRequests *struct {
				TotalCount *int `json:"totalCount"`
				Edges      []struct {
					Node struct {
						PK        int `json:"pk"`
						Recipient *struct {
							PK int `json:"pk"`
						} `json:"recipient"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"feedbackRequests"`
		}
		err := api.graphQL("listFeedbackRequests", "/feedback/", listFeedbackRequestsQuery, map[string]any{
			"profileId": profileID,
			"first":     feedbackRequestListPageSize,
			"offset":    received,
		}, &data)
		if err != nil {
			return nil, err
		}
		if data.FeedbackRequests == nil || data.FeedbackRequests.TotalCount == nil {
			return nil, fmt.Errorf("GraphQL listFeedbackRequests returned no feedback request list with totalCount for user %d", profileID)
		}
		for _, edge := range data.FeedbackRequests.Edges {
			if edge.Node.PK == 0 {
				return nil, fmt.Errorf("feedback request list of user %d has a request with no pk", profileID)
			}
			if edge.Node.Recipient == nil || edge.Node.Recipient.PK != profileID {
				return nil, fmt.Errorf("feedback request %d is in the list of user %d but its recipient is a different user; the list does not obey the recipient filter", edge.Node.PK, profileID)
			}
			seen[edge.Node.PK] = true
		}
		received += len(data.FeedbackRequests.Edges)
		total := *data.FeedbackRequests.TotalCount
		if received >= total || len(data.FeedbackRequests.Edges) == 0 {
			if len(seen) != total {
				return nil, fmt.Errorf("feedback request list of user %d returned %d different requests of %d; the list is not stable or it changed during the sync, run it again", profileID, len(seen), total)
			}
			break
		}
	}
	ids := make([]int, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids, nil
}

func closeFeedbackPage(requestID int, what string, pageInfo **gqlPageInfo) error {
	if *pageInfo == nil {
		return fmt.Errorf("feedback request %d has no page information for its %s", requestID, what)
	}
	if (*pageInfo).HasNextPage {
		return fmt.Errorf("feedback request %d has more %s than one request returns; the feedback request is not complete", requestID, what)
	}
	*pageInfo = nil
	return nil
}

func checkFeedbackRequestComplete(request *gqlFeedbackRequest) error {
	if request.NumResponsesSubmitted == nil || request.CanViewResponses == nil || request.CanViewAllResponses == nil {
		return fmt.Errorf("feedback request %d does not say how many responses it has or if they are visible", request.PK)
	}
	if err := closeFeedbackPage(request.PK, "responders", &request.Responders.PageInfo); err != nil {
		return err
	}
	if request.QuestionSet != nil {
		if err := closeFeedbackPage(request.PK, "questions", &request.QuestionSet.Questions.PageInfo); err != nil {
			return err
		}
	}
	if err := closeFeedbackPage(request.PK, "responses", &request.Feedbacks.PageInfo); err != nil {
		return err
	}
	submitted := 0
	for i := range request.Feedbacks.Edges {
		feedback := &request.Feedbacks.Edges[i].Node
		if feedback.SubmittedTimestamp != nil && *feedback.SubmittedTimestamp != "" {
			submitted++
		}
		if err := closeFeedbackPage(request.PK, "response comments", &feedback.Comments.PageInfo); err != nil {
			return err
		}
		if err := closeFeedbackPage(request.PK, "answers", &feedback.Answers.PageInfo); err != nil {
			return err
		}
		for j := range feedback.Answers.Edges {
			answer := &feedback.Answers.Edges[j].Node
			if err := closeFeedbackPage(request.PK, "answer comments", &answer.Comments.PageInfo); err != nil {
				return err
			}
			if answer.Choices != nil {
				if err := closeFeedbackPage(request.PK, "answer choices", &answer.Choices.PageInfo); err != nil {
					return err
				}
			}
		}
	}
	if *request.CanViewAllResponses && submitted < *request.NumResponsesSubmitted {
		return fmt.Errorf("feedback request %d has %d submitted responses but 7Geese returned %d; the responses are not complete", request.PK, *request.NumResponsesSubmitted, submitted)
	}
	if request.CuratedReport != nil {
		return closeCuratedReportPages(request.CuratedReport, func(what string, pageInfo **gqlPageInfo) error {
			return closeFeedbackPage(request.PK, what, pageInfo)
		})
	}
	return nil
}

func fetchFeedbackRequest(api *syncAPI, requestID int) (*gqlFeedbackRequest, error) {
	var data struct {
		FeedbackRequest *gqlFeedbackRequest `json:"feedbackRequest"`
	}
	err := api.graphQL("getFeedbackRequest", "/feedback/", getFeedbackRequestQuery, map[string]any{
		"pk": requestID,
	}, &data)
	if err != nil {
		return nil, err
	}
	if data.FeedbackRequest == nil {
		return nil, fmt.Errorf("feedback request %d is in the feedback list or in a review but its content is not visible", requestID)
	}
	if data.FeedbackRequest.PK != requestID {
		return nil, fmt.Errorf("feedback request %d was requested but feedback request %d was returned", requestID, data.FeedbackRequest.PK)
	}
	if err := checkFeedbackRequestComplete(data.FeedbackRequest); err != nil {
		return nil, err
	}
	return data.FeedbackRequest, nil
}

func fetchFeedbackRequests(api *syncAPI, profileID int, reviewRequestIDs []int) ([]store.Record, error) {
	listed, err := fetchFeedbackRequestIDs(api, profileID)
	if err != nil {
		return nil, err
	}
	wanted := map[int]bool{}
	for _, id := range listed {
		wanted[id] = true
	}
	for _, id := range reviewRequestIDs {
		wanted[id] = true
	}
	ids := make([]int, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	requests, err := fetchEach(ids, func(id int) (*gqlFeedbackRequest, error) {
		return fetchFeedbackRequest(api, id)
	})
	if err != nil {
		return nil, err
	}
	records := make([]store.Record, 0, len(requests))
	for i, request := range requests {
		data, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("encoding feedback request %d: %w", ids[i], err)
		}
		records = append(records, store.Record{ID: strconv.Itoa(ids[i]), Data: data})
	}
	return records, nil
}
