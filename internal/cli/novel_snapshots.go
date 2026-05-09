// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"7geese-cli/internal/config"
	"7geese-cli/internal/store"
)

// --- GraphQL types for review snapshots ---

type gqlSnapshotSummary struct {
	PK        int    `json:"pk"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	SnapshotGroup struct {
		PK    int    `json:"pk"`
		Title string `json:"title"`
	} `json:"snapshotGroup"`
}

type gqlSnapshotItem struct {
	Type  string `json:"type"`
	PK    int    `json:"pk"`
	Title string `json:"title"`
}

type gqlSnapshotSection struct {
	PK    int    `json:"pk"`
	Title string `json:"title"`
	Items struct {
		Edges []struct {
			Node gqlSnapshotItem `json:"node"`
		} `json:"edges"`
	} `json:"items"`
}

type gqlSnapshotAnswer struct {
	TypeName    string   `json:"__typename"`
	Answer      string   `json:"answer"`
	RangeAnswer *float64 `json:"rangeAnswer"`
	Choices     struct {
		Edges []struct {
			Node struct {
				Option struct {
					Title string `json:"title"`
				} `json:"option"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"choices"`
	Item struct {
		PK   int    `json:"pk"`
		Type string `json:"type"`
	} `json:"item"`
	Responder struct {
		PK       int    `json:"pk"`
		FullName string `json:"fullName"`
	} `json:"responder"`
}

type gqlSnapshotFull struct {
	PK            int    `json:"pk"`
	WorkflowState int    `json:"workflowState"`
	StartDate     string `json:"startDate"`
	EndDate       string `json:"endDate"`
	Position      string `json:"position"`
	Manager       *struct {
		FullName string `json:"fullName"`
	} `json:"manager"`
	SnapshotGroup struct {
		PK       int    `json:"pk"`
		Title    string `json:"title"`
		Sections struct {
			Edges []struct {
				Node gqlSnapshotSection `json:"node"`
			} `json:"edges"`
		} `json:"sections"`
	} `json:"snapshotGroup"`
	Answers struct {
		Edges []struct {
			Node gqlSnapshotAnswer `json:"node"`
		} `json:"edges"`
	} `json:"answers"`
	PeerFeedbackRequest *gqlPeerFeedbackRequest `json:"peerFeedbackRequest"`
}

type gqlPeerFeedbackRequest struct {
	PK    int    `json:"pk"`
	Title string `json:"title"`
	CuratedReport *struct {
		PublishedDatetime       string `json:"publishedDatetime"`
		SortedCuratedQuestions struct {
			Edges []struct {
				Node struct {
					Comment string          `json:"comment"`
					Question json.RawMessage `json:"question"`
					Answers  struct {
						Edges []struct {
							Node struct {
								Answer string `json:"answer"`
							} `json:"node"`
						} `json:"edges"`
					} `json:"answers"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"sortedCuratedQuestions"`
	} `json:"curatedReport"`
}

// listSnapshotsQuery uses the HAR-confirmed getReviewOverviewPastReviews pattern.
// state=4 is "completed"; we fetch all completed snapshots with no pkNin exclusion
// by passing a non-existent pk (0) so nothing is excluded.
const listSnapshotsQuery = `
query getReviewOverviewPastReviews($profileId: Int!, $first: Int!, $offset: Int!, $completedState: Int!) {
  snapshots(
    orderBy: "-created"
    forUser: $profileId
    first: $first
    offset: $offset
    state: $completedState
  ) {
    totalCount
    edges {
      node {
        pk
        startDate
        endDate
        snapshotGroup {
          pk
          title
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}
`

const getSingleSnapshotQuery = `
fragment oooFragment on OneOnOneNode {
  pk
  startTime
  completedTime
  __typename
}

fragment snapshotProfileFragment on UserProfileNode {
  pk
  fullName
  profileImageUrl
  __typename
}

fragment snapshotObjectiveFragment on ObjectiveNode {
  pk
  name
  status: overallAssessmentStatus
  progress
  startingValue
  currentValue
  targetValue
  measurementType
  objectiveType
  format
  weight
  grade
  canChange(changedFields: [{fieldName: "grade"}])
  closed
  startDate
  dueDatetime
  lastCheckin {
    pk
    message
    user {
      ...snapshotProfileFragment
      __typename
    }
    updated
    __typename
  }
  owners(first: 20) {
    edges {
      node {
        ...snapshotProfileFragment
        __typename
      }
      __typename
    }
    __typename
  }
  keyResults(first: 20) {
    edges {
      node {
        name
        pk
        progress
        measurementType
        currentValue
        startingValue
        targetValue
        weight
        owners(first: 20) {
          edges {
            node {
              ...snapshotProfileFragment
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  __typename
}

fragment snapshotSection on SectionNode {
  pk
  title
  description
  descriptionDataType
  managerOnly
  items(first: 90) {
    edges {
      node {
        ... on TextQuestionItemNode {
          type
          pk
          title
          description
          descriptionDataType
          shortInput
          canAnswer
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          ordinal
          __typename
        }
        ... on RangeQuestionItemNode {
          type
          pk
          title
          description
          descriptionDataType
          start
          end
          startText
          endText
          canAnswer
          allowDecimals
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          ordinal
          __typename
        }
        ... on LikertQuestionItemNode {
          type
          pk
          title
          description
          descriptionDataType
          optionCount
          style
          canAnswer
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          ordinal
          __typename
        }
        ... on MultiOptionQuestionItemNode {
          type
          pk
          title
          description
          descriptionDataType
          singleChoice
          options(first: 50) {
            edges {
              node {
                pk
                title
                __typename
              }
              __typename
            }
            __typename
          }
          canAnswer
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          ordinal
          __typename
        }
        ... on BooleanQuestionItemNode {
          type
          pk
          title
          description
          descriptionDataType
          trueText
          falseText
          canAnswer
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          ordinal
          __typename
        }
        ... on RatingItemNode {
          pk
          type
          title
          description
          descriptionDataType
          canAnswer
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          rating: effectiveRating {
            pk
            name
            title
            description
            options(first: 50) {
              edges {
                node {
                  pk
                  label
                  value
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        ... on CalculatedRatingItemNode {
          pk
          type
          title
          description
          descriptionDataType
          canAnswer
          members(first: 50) {
            totalCount
            edges {
              node {
                pk
                weighting
                targetRatingItem {
                  pk
                  type
                  ratingId
                  title
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          managerAnswerVisibility
          managerCanAnswer
          employeeCanAnswer
          managerAnswerRequired
          employeeAnswerRequired
          __typename
        }
        ... on ObjectivesInfoItemNode {
          pk
          type
          showStatus
          showProgress
          showWeight
          showResult
          allowDecimals
          startValue
          endValue
          ordinal
          canAnswer
          ownedObjectives(first: 50) {
            edges {
              node {
                ...snapshotObjectiveFragment
                __typename
              }
              __typename
            }
            __typename
          }
          stakeholderObjectives(first: 50) {
            edges {
              node {
                ...snapshotObjectiveFragment
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        ... on RecognitionInfoItemNode {
          pk
          type
          canAnswer
          ordinal
          recognitionSent(first: 50, includeTeamRecognitions: true) {
            totalCount
            edges {
              node {
                pk
                message
                sender {
                  fullName
                  __typename
                }
                userRecipients(first: 50) {
                  edges {
                    node {
                      recipient {
                        fullName
                        __typename
                      }
                      __typename
                    }
                    __typename
                  }
                  __typename
                }
                teamRecipients(first: 50) {
                  edges {
                    node {
                      fullName: name
                      __typename
                    }
                    __typename
                  }
                  __typename
                }
                awardedDate
                badge {
                  image
                  name
                  pk
                  __typename
                }
                recognitionWebViewUrl
                __typename
              }
              __typename
            }
            __typename
          }
          recognitionReceived(first: 50, includeTeamRecognitions: true) {
            totalCount
            edges {
              node {
                pk
                message
                sender {
                  fullName
                  __typename
                }
                userRecipients(first: 50) {
                  edges {
                    node {
                      recipient {
                        fullName
                        __typename
                      }
                      __typename
                    }
                    __typename
                  }
                  __typename
                }
                teamRecipients(first: 50) {
                  edges {
                    node {
                      fullName: name
                      __typename
                    }
                    __typename
                  }
                  __typename
                }
                awardedDate
                badge {
                  image
                  name
                  pk
                  __typename
                }
                recognitionWebViewUrl
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        ... on FeedbackInfoItemNode {
          pk
          type
          canAnswer
          ordinal
          feedbackRequests(first: 30) {
            edges {
              node {
                pk
                title
                type
                creator {
                  fullName
                  __typename
                }
                adminsCanView
                directManagersCanView
                indirectManagersCanView
                respondentsNamesVisibleTo
                participantsCanView
                reportingTreeCanView
                numResponsesSubmitted
                responders(first: 30) {
                  totalCount
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          threesixtyFeedbackRequests(first: 30) {
            edges {
              node {
                pk
                title
                type
                creator {
                  fullName
                  __typename
                }
                curatedReport {
                  id
                  publishedDatetime
                  __typename
                }
                respondentsNamesVisibleTo
                adminsCanView
                directManagersCanView
                indirectManagersCanView
                participantsCanView
                reportingTreeCanView
                numResponsesSubmitted
                responders(first: 30) {
                  totalCount
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        ... on OneOnOneInfoItemNode {
          pk
          type
          canAnswer
          ordinal
          completedOneonones(first: 50) {
            edges {
              node {
                pk
                name
                updatedTime
                adminsCanView
                managersCanView
                managementTreesCanView
                participant {
                  ...snapshotProfileFragment
                  profileUrl
                  __typename
                }
                facilitator {
                  ...snapshotProfileFragment
                  profileUrl
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  __typename
}

fragment talentAttributesInReviewsFragment on TalentIndicatorInterface {
  pk
  indicatorType
  effectiveStart
  changedBy {
    pk
    fullName
    profileImageUrl
    __typename
  }
  notes
  snapshotId
  __typename
}

fragment talentAttributeTypesInReviewsFragment on TalentIndicatorUnionNode {
  ... on ReadyForPromotionNode {
    ...talentAttributesInReviewsFragment
    months
    value
    __typename
  }
  ... on RetentionRiskNode {
    ...talentAttributesInReviewsFragment
    value
    __typename
  }
  ... on SuccessorIdentifiedNode {
    ...talentAttributesInReviewsFragment
    value
    __typename
  }
  __typename
}

fragment textCuratedReportAnswerFragment on TextAnswerNode {
  ... on TextAnswerNode {
    pk
    answer
    feedback {
      responder {
        profile {
          pk
          fullName
          profileImageUrl
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  __typename
}

fragment textCuratedReportQuestionFragment on TextQuestionNode {
  pk
  type
  title
  description
  isAnswerRequired
  __typename
}

fragment rangeCuratedReportQuestionFragment on RangeQuestionNode {
  pk
  type
  title
  description
  isAnswerRequired
  start
  startText
  end
  endText
  answerSummary
  answers {
    totalCount
    __typename
  }
  __typename
}

fragment multiOptionCuratedReportQuestionFragment on MultiOptionQuestionNode {
  pk
  type
  title
  description
  isAnswerRequired
  singleChoice
  options(first: 50) {
    edges {
      node {
        pk
        title
        __typename
      }
      __typename
    }
    __typename
  }
  answerSummary
  answers {
    totalCount
    __typename
  }
  __typename
}

fragment npsCuratedReportQuestionFragment on NpsQuestionNode {
  pk
  type
  title
  description
  isAnswerRequired
  answerSummary
  answers {
    totalCount
    __typename
  }
  __typename
}

fragment likertCuratedReportQuestionFragment on LikertQuestionNode {
  pk
  type
  title
  description
  isAnswerRequired
  style
  optionCount
  answerSummary
  answers {
    totalCount
    __typename
  }
  __typename
}

fragment snapshotsSingleFragment on SnapshotNode {
  pk
  workflowState: state
  startDate
  endDate
  canChange
  canTransitionToNextState
  canTransitionToPreviousState
  transitionPermissions
  canViewAnswers
  employeeAnswersDue
  employeeHasPresubmittedAnswers
  managerAnswersDue
  managerHasPresubmittedAnswers
  canCurrentUserSubmitOrPresubmitAnswers
  showManagerAnswers
  position
  employeeAcknowledgment
  managerAcknowledgment
  employeeAcknowledgmentComment
  approvals(first: 50, orderBy: "-created") {
    edges {
      node {
        pk
        previousState
        newState
        approvedBy {
          fullName
          profileImageUrl
          __typename
        }
        note
        created
        __typename
      }
      __typename
    }
    __typename
  }
  assignedToProfiles(first: 50) {
    totalCount
    edges {
      node {
        ...snapshotProfileFragment
        __typename
      }
      __typename
    }
    __typename
  }
  manager {
    ...snapshotProfileFragment
    user {
      email
      __typename
    }
    __typename
  }
  skipLevelManager {
    ...snapshotProfileFragment
    __typename
  }
  admin {
    ...snapshotProfileFragment
    __typename
  }
  forUser {
    ...snapshotProfileFragment
    canChangeTalentIndicators
    canViewGrowthPlanTab
    reportsTo {
      pk
      __typename
    }
    user {
      email
      __typename
    }
    __typename
  }
  oneonone {
    ...oooFragment
    __typename
  }
  ignoredFeedbackRequests(first: 25) {
    edges {
      node {
        pk
        __typename
      }
      __typename
    }
    __typename
  }
  ignoredOneonones(first: 25) {
    edges {
      node {
        pk
        __typename
      }
      __typename
    }
    __typename
  }
  ignoredRecognitions(first: 25) {
    edges {
      node {
        pk
        __typename
      }
      __typename
    }
    __typename
  }
  draftTalentIndicators: talentIndicators(first: 3, draft: true) {
    edges {
      node {
        ... on ReadyForPromotionNode {
          indicatorType
          notes
          months
          value
          __typename
        }
        ... on RetentionRiskNode {
          indicatorType
          notes
          value
          __typename
        }
        ... on SuccessorIdentifiedNode {
          indicatorType
          notes
          value
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  snapshotPublishedTalentIndicators: talentIndicators(first: 9, draft: false) {
    edges {
      node {
        ...talentAttributeTypesInReviewsFragment
        __typename
      }
      __typename
    }
    __typename
  }
  snapshotGroup {
    pk
    title
    managerAnswerSharing
    employeeRoleAssessmentEnabled
    managerRoleAssessmentEnabled
    talentIndicatorUpdateEnabled
    employeeAcknowledgementCommentEnabled
    oneononeTemplate {
      pk
      name
      __typename
    }
    peerInputSettings {
      nominationInstructions
      nominationTitle
      anonymity
      reportSharing
      directManagersCanView
      managementTreeCanView
      respondersCanView
      adminsCanView
      isLocked
      __typename
    }
    snapshotStates(first: 10) {
      edges {
        node {
          state
          settings {
            title
            employeeCanAnswer
            managerCanAnswer
            employeeCanPresubmit
            managerCanPresubmit
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    sections(first: 21) {
      edges {
        node {
          ...snapshotSection
          __typename
        }
        __typename
      }
      __typename
    }
    singleFeedbackInfo {
      includedFeedbackRequests(first: 100) {
        totalCount
        __typename
      }
      includedThreesixtyFeedbackRequests(first: 100) {
        totalCount
        __typename
      }
      __typename
    }
    __typename
  }
  answers(first: 300) {
    edges {
      node {
        ... on TextItemAnswerNode {
          answer
          answerDataType
          item {
            pk
            type
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on RangeItemAnswerNode {
          rangeAnswer: answer
          item {
            pk
            type
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on LikertItemAnswerNode {
          likertAnswer: answer
          item {
            pk
            type
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on BooleanItemAnswerNode {
          booleanAnswer: answer
          item {
            pk
            type
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on MultiOptionItemAnswerNode {
          choices(first: 50) {
            edges {
              node {
                option {
                  pk
                  title
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          item {
            pk
            type
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on RatingItemAnswerNode {
          pk
          choiceValue
          comment
          item {
            pk
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on CalculatedRatingItemAnswerNode {
          value
          item {
            pk
            type
            maxValue
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        ... on ObjectiveInfoItemAnswerNode {
          grades {
            grade
            targetObjectiveId
            __typename
          }
          item {
            pk
            startValue
            endValue
            allowDecimals
            __typename
          }
          responder {
            ...snapshotProfileFragment
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  hasDeletedPeerFeedbackRequest
  peerFeedbackStatus
  peerFeedbackRequest {
    pk
    title
    canChange
    numResponsesSubmitted
    isOverdue
    allResponders {
      totalCount
      __typename
    }
    canAddResponders
    respondentsNamesVisibleTo
    questionSet {
      questionGroups(first: 80) {
        edges {
          node {
            title
            description
            questions(first: 80) {
              edges {
                node {
                  ... on TextQuestionNode {
                    pk
                    __typename
                  }
                  ... on RangeQuestionNode {
                    pk
                    __typename
                  }
                  ... on MultiOptionQuestionNode {
                    pk
                    __typename
                  }
                  ... on NpsQuestionNode {
                    pk
                    __typename
                  }
                  ... on LikertQuestionNode {
                    pk
                    __typename
                  }
                  __typename
                }
                __typename
              }
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    curatedReport {
      pk
      state
      canChange
      publishedDatetime
      intro
      outro
      creator {
        pk
        fullName
        profileImageUrl
        __typename
      }
      respondentsNamesVisible
      newResponseCount
      sortedCuratedQuestions(first: 80) {
        edges {
          node {
            pk
            comment
            includeAnswers
            answers(first: 80) {
              totalCount
              edges {
                node {
                  ...textCuratedReportAnswerFragment
                  __typename
                }
                __typename
              }
              __typename
            }
            question {
              ...textCuratedReportQuestionFragment
              ...rangeCuratedReportQuestionFragment
              ...multiOptionCuratedReportQuestionFragment
              ...npsCuratedReportQuestionFragment
              ...likertCuratedReportQuestionFragment
              __typename
            }
            __typename
          }
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  __typename
}

query getSingleSnapshot($snapshotId: Int!) {
  snapshot(pk: $snapshotId) {
    ...snapshotsSingleFragment
    __typename
  }
}
`

func snapshotGraphQLRequest(cfg *config.Config, opname, query string, variables map[string]any) (*http.Request, error) {
	body, _ := json.Marshal(map[string]any{
		"operationName": opname,
		"variables":     variables,
		"query":         query,
	})
	req, err := http.NewRequest("POST", cfg.BaseURL+"/graphql?opname="+opname, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	referer := cfg.BaseURL + "/reviews/"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sgsession4="+cfg.SevengeeseSession+"; sgcsrftoken4="+cfg.SevengeeseCSRF)
	req.Header.Set("X-CSRFToken", cfg.SevengeeseCSRF)
	req.Header.Set("Referer", referer)
	req.Header.Set("X-HREF", referer)
	req.Header.Set("Origin", cfg.BaseURL)
	return req, nil
}

func fetchSnapshotList(cfg *config.Config, profileID int) ([]gqlSnapshotSummary, error) {
	var all []gqlSnapshotSummary
	offset := 0
	const limit = 50
	for {
		req, err := snapshotGraphQLRequest(cfg, "getReviewOverviewPastReviews", listSnapshotsQuery, map[string]any{
			"profileId":      profileID,
			"first":          limit,
			"offset":         offset,
			"completedState": 4,
		})
		if err != nil {
			return nil, err
		}
		resp, err := syncHTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Data struct {
				Snapshots struct {
					Edges []struct {
						Node gqlSnapshotSummary `json:"node"`
					} `json:"edges"`
				} `json:"snapshots"`
			} `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&envelope)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		batch := envelope.Data.Snapshots.Edges
		for _, e := range batch {
			all = append(all, e.Node)
		}
		if len(batch) < limit {
			break
		}
		offset += limit
	}
	return all, nil
}

func fetchSingleSnapshot(cfg *config.Config, snapshotID int) (*gqlSnapshotFull, error) {
	req, err := snapshotGraphQLRequest(cfg, "getSingleSnapshot", getSingleSnapshotQuery, map[string]any{
		"snapshotId": snapshotID,
	})
	if err != nil {
		return nil, err
	}
	resp, err := syncHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Data struct {
			Snapshot gqlSnapshotFull `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return &envelope.Data.Snapshot, nil
}

func syncUserSnapshots(flags *rootFlags, db *store.Store, profileID int, force bool) (int, error) {
	cfg, err := config.Load(flags.configPath)
	if err != nil || cfg.SevengeeseSession == "" {
		return 0, fmt.Errorf("no auth configured")
	}
	summaries, err := fetchSnapshotList(cfg, profileID)
	if err != nil {
		return 0, fmt.Errorf("listing snapshots: %w", err)
	}
	synced := 0
	for _, s := range summaries {
		id := fmt.Sprintf("%d", s.PK)
		if !force {
			existing, _ := db.Get("user_snapshots", id)
			if existing != nil {
				continue
			}
		}
		snap, err := fetchSingleSnapshot(cfg, s.PK)
		if err != nil {
			continue
		}
		data, err := json.Marshal(snap)
		if err != nil {
			continue
		}
		if err := db.Upsert("user_snapshots", id, json.RawMessage(data)); err != nil {
			continue
		}
		synced++
	}
	return synced, nil
}

// snapshotStateName maps the numeric workflowState to a human-readable label.
func snapshotStateName(state int) string {
	switch state {
	case 0:
		return "employee_input"
	case 1:
		return "manager_input"
	case 2:
		return "skip_level_approval"
	case 3:
		return "admin_approval"
	case 4:
		return "completed"
	case 5:
		return "acknowledgment"
	default:
		return "unknown"
	}
}

// stripHTMLTags converts HTML to readable plain text by replacing block tags
// with newlines and stripping all remaining tags.
func stripHTMLTags(s string) string {
	// Replace block-level tags with newlines before stripping.
	for _, tag := range []string{"</p>", "<br>", "<br/>", "<br />", "</li>", "</div>"} {
		s = strings.ReplaceAll(s, tag, "\n")
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	// Collapse runs of blank lines to a single blank line.
	lines := strings.Split(b.String(), "\n")
	var out []string
	blank := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			blank++
			if blank <= 1 {
				out = append(out, "")
			}
		} else {
			blank = 0
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
