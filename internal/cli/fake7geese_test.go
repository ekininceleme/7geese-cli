// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"7geese-cli/internal/store"
)

const (
	fakeSession = "valid-session"
	fakeCSRF    = "csrf-token"

	fakeMe        = 100
	fakeManager   = 200
	fakeReport    = 300
	fakeColleague = 400
)

var fakeNow = time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)

type fakeCheckin struct {
	PK       int
	Message  string
	Created  string
	Author   string
	Progress float64
}

type fakeObjective struct {
	PK       int
	Name     string
	Closed   bool
	Type     int
	Role     int
	Progress float64
	Due      string
	Checkins []fakeCheckin
}

type fakeReview struct {
	State int
	Data  map[string]any
}

type fakeFeedbackAnswer struct {
	TypeName string
	Question string
	Hint     string
	Text     string
	Number   int
	Options  []string
}

type fakeFeedbackResponse struct {
	PK        int
	From      string
	Submitted string
	Comment   string
	Answers   []fakeFeedbackAnswer
}

type fakeFeedbackRequest struct {
	PK         int
	Recipient  int
	Title      string
	Status     string
	Created    string
	Asked      []string
	Submitted  int
	CanViewAll bool
	Listed     bool
	Responses  []fakeFeedbackResponse
}

type fakeFeedbackEntry struct {
	Listed    bool
	Hidden    bool
	Recipient int
	Data      map[string]any
}

func fakeClosedPage(edges []any) map[string]any {
	return map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": edges}
}

func fakeFeedback(request fakeFeedbackRequest) fakeFeedbackEntry {
	responders := []any{}
	for i, name := range request.Asked {
		responders = append(responders, map[string]any{"node": map[string]any{
			"pk": request.PK*10 + i, "state": 2, "profile": fakeGQLUser(i+1, name),
		}})
	}
	feedbacks := []any{}
	for _, response := range request.Responses {
		answers := []any{}
		for i, answer := range response.Answers {
			question := map[string]any{
				"__typename": "TextQuestionNode", "pk": i + 1, "type": "text",
				"title": answer.Question, "description": answer.Hint, "ordinal": i,
			}
			node := map[string]any{
				"__typename": answer.TypeName, "pk": response.PK*10 + i,
				"question": question, "comments": fakeClosedPage([]any{}),
			}
			switch answer.TypeName {
			case "RangeAnswerNode":
				node["number"] = answer.Number
				question["__typename"] = "RangeQuestionNode"
				question["start"], question["startText"] = 1, "Poor"
				question["end"], question["endText"] = 5, "Great"
			case "MultiOptionAnswerNode":
				choices := []any{}
				for _, option := range answer.Options {
					choices = append(choices, map[string]any{"node": map[string]any{"option": map[string]any{"title": option}}})
				}
				node["choices"] = fakeClosedPage(choices)
			default:
				node["text"] = answer.Text
			}
			answers = append(answers, map[string]any{"node": node})
		}
		responder := map[string]any{"profile": nil}
		if response.From != "" {
			responder["profile"] = fakeGQLUser(1, response.From)
		}
		var submitted any
		if response.Submitted != "" {
			submitted = response.Submitted
		}
		comments := []any{}
		if response.Comment != "" {
			comments = append(comments, map[string]any{"node": map[string]any{
				"pk": response.PK*10 + 9, "created": "2026-05-22T09:00:00+00:00",
				"comment": response.Comment, "creator": fakeGQLUser(fakeMe, "Alex Morgan"),
			}})
		}
		feedbacks = append(feedbacks, map[string]any{"node": map[string]any{
			"pk": response.PK, "state": 2, "created": request.Created, "submittedTimestamp": submitted,
			"responder": responder, "answers": fakeClosedPage(answers), "comments": fakeClosedPage(comments),
		}})
	}
	return fakeFeedbackEntry{Listed: request.Listed, Recipient: request.Recipient, Data: map[string]any{
		"pk": request.PK, "title": request.Title, "description": "<p>Please give feedback</p>",
		"type": 0, "state": 1, "statusByRecipient": request.Status,
		"created": request.Created, "sentDate": request.Created, "dueDatetime": nil, "isArchived": false,
		"respondentsNamesVisibleTo": 2, "numResponsesSubmitted": request.Submitted,
		"canViewResponses": true, "canViewAllResponses": request.CanViewAll,
		"creator":       fakeGQLUser(fakeManager, "Blake Chen"),
		"recipient":     fakeGQLUser(request.Recipient, "Alex Morgan"),
		"responders":    fakeClosedPage(responders),
		"questionSet":   map[string]any{"questions": fakeClosedPage([]any{})},
		"feedbacks":     fakeClosedPage(feedbacks),
		"curatedReport": nil,
	}}
}

func linkFeedback(review fakeReview, requests, threesixty []int) fakeReview {
	edges := func(pks []int) []any {
		linked := []any{}
		for _, pk := range pks {
			linked = append(linked, map[string]any{"node": map[string]any{"pk": pk, "title": fmt.Sprintf("Feedback %d", pk)}})
		}
		return linked
	}
	sections := review.Data["snapshotGroup"].(map[string]any)["sections"].(map[string]any)
	items := sections["edges"].([]any)[0].(map[string]any)["node"].(map[string]any)["items"].(map[string]any)
	items["edges"] = append(items["edges"].([]any), map[string]any{"node": map[string]any{
		"type": "feedback_info", "pk": review.Data["pk"].(int)*100 + 99,
		"feedbackRequests":           fakeClosedPage(edges(requests)),
		"threesixtyFeedbackRequests": fakeClosedPage(edges(threesixty)),
	}})
	return review
}

type fake7Geese struct {
	t      *testing.T
	server *httptest.Server

	mu               sync.Mutex
	userID           int
	lists            map[string][]map[string]any
	single           map[string]map[string]any
	requiredParams   map[string][2]string
	totalCountDelta  map[string]int
	unstableLists    map[string]bool
	ignoreOffset     map[string]bool
	stuckCheckins    bool
	failures         map[string]int
	objectives       map[int]map[string][]fakeObjective
	reviews          map[int][]fakeReview
	feedback         []fakeFeedbackEntry
	meetings         map[int]map[string]any
	recognitionFeed  []map[string]any
	notificationFeed []map[string]any
	recognitionWraps int
	requests         map[string]int
}

func fakePerson(id int, first, last string) map[string]any {
	return map[string]any{
		"id":   id,
		"user": map[string]any{"first_name": first, "last_name": last, "email": strings.ToLower(first) + "@example.com"},
	}
}

func fakeProfile(id int, first, last string, managerID int) map[string]any {
	profile := fakePerson(id, first, last)
	profile["position"] = "Engineer"
	profile["hire_date"] = "2020-01-02"
	profile["resource_uri"] = profileURI(id)
	profile["reports_to"] = nil
	if managerID != 0 {
		profile["reports_to"] = profileURI(managerID)
	}
	return profile
}

func fakeMeetingItem(id int, start string, status int, creator, target map[string]any) map[string]any {
	return map[string]any{"id": id, "start_datetime": start, "status": status, "creator": creator, "target": target}
}

type fakeQuestion struct {
	PK    int
	Title string
	Notes [][2]string
}

func fakeGQLUser(pk int, name string) map[string]any {
	return map[string]any{"pk": pk, "fullName": name}
}

func fakeFinalizedMeeting(questions []fakeQuestion, comments [][2]string) map[string]any {
	questionEdges := []any{}
	for _, question := range questions {
		noteEdges := []any{}
		for i, note := range question.Notes {
			noteEdges = append(noteEdges, map[string]any{"node": map[string]any{
				"pk": question.PK*100 + i, "creator": fakeGQLUser(1, note[0]), "content": note[1],
			}})
		}
		questionEdges = append(questionEdges, map[string]any{"node": map[string]any{
			"pk": question.PK, "content": question.Title, "description": "",
			"notes": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": noteEdges},
		}})
	}
	commentEdges := []any{}
	for i, comment := range comments {
		commentEdges = append(commentEdges, map[string]any{"node": map[string]any{
			"pk": i + 1, "content": comment[1], "creator": fakeGQLUser(1, comment[0]),
		}})
	}
	return map[string]any{
		"name":        "1:1",
		"facilitator": fakeGQLUser(fakeManager, "Blake Chen"),
		"participant": fakeGQLUser(fakeMe, "Alex Morgan"),
		"questionSet": map[string]any{"questions": map[string]any{
			"pageInfo": map[string]any{"hasNextPage": false}, "edges": questionEdges,
		}},
		"comments": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": commentEdges},
	}
}

type fakeAnswer struct {
	ItemPK      int
	ResponderPK int
	TypeName    string
	Text        string
}

func fakeSnapshot(pk, state int, title, start, end string, itemPKs []int, answers []fakeAnswer) fakeReview {
	itemEdges := []any{}
	for _, itemPK := range itemPKs {
		itemEdges = append(itemEdges, map[string]any{"node": map[string]any{
			"type": "text", "pk": itemPK, "title": fmt.Sprintf("Question %d", itemPK),
		}})
	}
	answerEdges := []any{}
	for _, answer := range answers {
		answerEdges = append(answerEdges, map[string]any{"node": map[string]any{
			"__typename": answer.TypeName,
			"answer":     answer.Text,
			"item":       map[string]any{"pk": answer.ItemPK, "type": "text"},
			"responder":  fakeGQLUser(answer.ResponderPK, "Responder"),
		}})
	}
	return fakeReview{State: state, Data: map[string]any{
		"pk":            pk,
		"workflowState": state,
		"startDate":     start,
		"endDate":       end,
		"position":      "Engineer",
		"manager":       map[string]any{"fullName": "Blake Chen"},
		"snapshotGroup": map[string]any{
			"pk": pk + 1000, "title": title,
			"sections": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": []any{map[string]any{"node": map[string]any{
				"pk": pk + 2000, "title": "Section",
				"items": map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": itemEdges},
			}}}},
		},
		"answers":             map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": answerEdges},
		"peerFeedbackRequest": nil,
	}}
}

func fakeCheckinHistory(count int) []fakeCheckin {
	newest := time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC)
	history := make([]fakeCheckin, 0, count)
	for i := 0; i < count; i++ {
		history = append(history, fakeCheckin{
			PK:       5000 + count - i,
			Message:  fmt.Sprintf("update %d", count-i),
			Created:  newest.AddDate(0, 0, -i).Format("2006-01-02T15:04:05+00:00"),
			Author:   "Alex Morgan",
			Progress: float64(count-i) / 10,
		})
	}
	return history
}

func fakeFeedItems(prefix string, count int) []map[string]any {
	newest := time.Date(2026, 6, 16, 8, 0, 0, 0, time.UTC)
	items := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, map[string]any{
			"id":        fmt.Sprintf("%s%d", prefix, count-i),
			"published": newest.Add(-time.Duration(i) * time.Hour).Format("2006-01-02T15:04:05Z"),
			"verb":      "recognize",
			"actor":     map[string]any{"displayName": "Blake Chen"},
			"target":    map[string]any{"displayName": "Alex Morgan"},
			"object":    map[string]any{"badgeName": "Star", "content": fmt.Sprintf("%s message %d", prefix, count-i)},
			"content":   fmt.Sprintf("%s content %d", prefix, count-i),
		})
	}
	return items
}

func fakeUserCheckin(id int, created, message string, objectiveID int, progress float64) map[string]any {
	return map[string]any{
		"id": id, "created": created, "message": message,
		"creator": map[string]any{"id": fakeMe},
		"metadata": map[string]any{"metadata": map[string]any{"objective": map[string]any{"snapshot": map[string]any{
			"id": objectiveID, "name": fmt.Sprintf("Objective %d", objectiveID), "progress": progress,
		}}}},
	}
}

func newFake7Geese(t *testing.T) *fake7Geese {
	t.Helper()
	me := fakePerson(fakeMe, "Alex", "Morgan")
	manager := fakePerson(fakeManager, "Blake", "Chen")
	report := fakePerson(fakeReport, "Casey", "Diaz")

	fillers := []fakeObjective{{PK: 14, Name: "Followed objective", Type: 2, Role: 3, Progress: 10, Due: "2026-12-31T23:59:59+00:00"}}
	for i := 0; i < 55; i++ {
		fillers = append(fillers, fakeObjective{
			PK: 2000 + i, Name: fmt.Sprintf("Old objective %d", i), Closed: true, Type: 3, Role: 3,
			Progress: 100, Due: "2024-12-31T23:59:59+00:00",
		})
	}

	longQuestionNotes := [][2]string{}
	for i := 1; i <= 12; i++ {
		longQuestionNotes = append(longQuestionNotes, [2]string{"Alex Morgan", fmt.Sprintf("note %d", i)})
	}

	f := &fake7Geese{
		t:      t,
		userID: fakeMe,
		lists: map[string][]map[string]any{
			"/api/v1/userprofile/": {
				fakeProfile(fakeMe, "Alex", "Morgan", fakeManager),
				fakeProfile(fakeManager, "Blake", "Chen", 0),
				fakeProfile(fakeReport, "Casey", "Diaz", fakeMe),
				fakeProfile(fakeColleague, "Devon", "Ellis", fakeManager),
				fakeProfile(500, "Sam", "Twin", fakeManager),
				fakeProfile(501, "Sam", "Twin", fakeManager),
			},
			"/api/v1/oneonones/": {
				fakeMeetingItem(1, "2026-03-02T10:00:00+00:00", 1, manager, me),
				fakeMeetingItem(2, "2026-04-06T10:00:00+00:00", 1, me, report),
				fakeMeetingItem(3, "2026-06-20T10:00:00+00:00", 0, manager, me),
				fakeMeetingItem(4, "2026-05-01T10:00:00+00:00", 0, manager, me),
			},
			"/api/v1/oneononenotes/": {
				{"id": 9001, "content": "note one"},
				{"id": 9002, "content": "note two"},
			},
			"/api/v1/performancecycles/": {{"id": 8001, "name": "Cycle"}},
			"/api/v1/recognitionbadges/": {
				{"id": 7001, "created": "2026-02-01T09:00:00+00:00", "sender": me, "recipient": manager, "badge": map[string]any{"name": "Star"}, "message": "thanks"},
				{"id": 7002, "created": "2026-05-01T09:00:00+00:00", "sender": me, "recipient": report, "badge": map[string]any{"name": "Team"}, "message": "well done"},
			},
			"/api/v1/checkins/": {
				fakeUserCheckin(6001, "2026-06-16T09:00:00.123456+00:00", "this week", 11, 25),
				fakeUserCheckin(6002, "2026-06-01T09:00:00+00:00", "earlier", 12, 40),
				fakeUserCheckin(6003, "2026-01-10T09:00:00+00:00", "january", 12, 5),
			},
		},
		single: map[string]map[string]any{},
		requiredParams: map[string][2]string{
			"/api/v1/performancecycles/": {"target", strconv.Itoa(fakeMe)},
			"/api/v1/recognitionbadges/": {"sender", strconv.Itoa(fakeMe)},
			"/api/v1/checkins/":          {"creator", strconv.Itoa(fakeMe)},
		},
		totalCountDelta: map[string]int{},
		unstableLists:   map[string]bool{},
		ignoreOffset:    map[string]bool{},
		failures:        map[string]int{},
		objectives: map[int]map[string][]fakeObjective{
			fakeMe: {
				"ownerOrStakeholder": {
					{PK: 11, Name: "Ship the product", Type: 1, Role: 1, Progress: 25, Due: "2026-12-31T23:59:59+00:00", Checkins: fakeCheckinHistory(250)},
					{PK: 12, Name: "Grow the team", Type: 2, Role: 2, Progress: 40, Due: "2026-09-30T23:59:59+00:00", Checkins: []fakeCheckin{
						{PK: 4001, Message: "hired one", Created: "2026-05-01T09:00:00+00:00", Author: "Blake Chen", Progress: 40},
					}},
					{PK: 13, Name: "Closed objective", Closed: true, Type: 1, Role: 1, Progress: 100, Due: "2025-12-31T23:59:59+00:00"},
				},
				"follower": fillers,
			},
			fakeReport: {
				"ownerOrStakeholder": {{PK: 31, Name: "Report objective", Type: 1, Progress: 60, Due: "2026-12-31T23:59:59+00:00"}},
			},
		},
		reviews: map[int][]fakeReview{
			fakeMe: {
				linkFeedback(fakeSnapshot(71, 4, "Q1 review", "2026-01-01", "2026-03-31", []int{711, 712}, []fakeAnswer{
					{ItemPK: 711, ResponderPK: fakeManager, TypeName: "TextItemAnswerNode", Text: "<p>Manager answer</p>"},
					{ItemPK: 711, ResponderPK: fakeMe, TypeName: "TextItemAnswerNode", Text: "<p>My answer</p>"},
					{ItemPK: 712, ResponderPK: fakeMe, TypeName: "TextItemAnswerNode", Text: ""},
				}), nil, []int{83}),
				linkFeedback(fakeSnapshot(72, 0, "H1 review", "2026-01-01", "2026-06-30", []int{721}, nil), []int{81}, nil),
			},
			fakeReport: {
				fakeSnapshot(73, 4, "2025 review", "2025-01-01", "2025-12-31", []int{731}, []fakeAnswer{
					{ItemPK: 731, ResponderPK: fakeReport, TypeName: "TextItemAnswerNode", Text: "Report answer"},
					{ItemPK: 731, ResponderPK: fakeMe, TypeName: "TextItemAnswerNode", Text: "Manager view"},
				}),
			},
		},
		feedback: []fakeFeedbackEntry{
			fakeFeedback(fakeFeedbackRequest{
				PK: 81, Recipient: fakeMe, Title: "Mid-Year feedback", Status: "COMPLETE", Created: "2026-05-18T10:00:00.123456+00:00",
				Asked: []string{"Devon Ellis", "Blake Chen"}, Submitted: 2, CanViewAll: true, Listed: true,
				Responses: []fakeFeedbackResponse{
					{PK: 811, From: "Devon Ellis", Submitted: "2026-05-20T09:00:00+00:00", Comment: "<p>Thanks</p>", Answers: []fakeFeedbackAnswer{
						{TypeName: "TextAnswerNode", Question: "What went well?", Hint: "<p>Give examples</p>", Text: "<p>Great work</p>"},
						{TypeName: "TextAnswerNode", Question: "What to improve?", Text: ""},
					}},
					{PK: 812, Submitted: "2026-05-21T09:00:00+00:00", Answers: []fakeFeedbackAnswer{
						{TypeName: "RangeAnswerNode", Question: "Rate the work", Number: 4},
						{TypeName: "MultiOptionAnswerNode", Question: "Strengths", Options: []string{"Speed", "Quality"}},
					}},
				},
			}),
			fakeFeedback(fakeFeedbackRequest{
				PK: 82, Recipient: fakeMe, Title: "Draft feedback", Status: "DRAFT", Created: "2025-11-02T09:00:00+00:00", Listed: true,
			}),
			fakeFeedback(fakeFeedbackRequest{
				PK: 83, Recipient: fakeMe, Title: "Q1 feedback", Status: "COMPLETE", Created: "2026-02-01T09:00:00+00:00",
				Asked: []string{"Blake Chen"}, Submitted: 1, CanViewAll: true,
				Responses: []fakeFeedbackResponse{
					{PK: 831, From: "Blake Chen", Submitted: "2026-02-03T09:00:00+00:00", Answers: []fakeFeedbackAnswer{
						{TypeName: "TextAnswerNode", Question: "What went well?", Text: "Good quarter"},
					}},
				},
			}),
		},
		meetings: map[int]map[string]any{
			1: fakeFinalizedMeeting([]fakeQuestion{
				{PK: 1, Title: "Wins?", Notes: [][2]string{{"Alex Morgan", "Shipped X"}, {"Blake Chen", "Great"}}},
				{PK: 2, Title: "Details?", Notes: longQuestionNotes},
				{PK: 3, Title: "", Notes: nil},
			}, [][2]string{{"Blake Chen", "Good meeting"}}),
			2: fakeFinalizedMeeting([]fakeQuestion{
				{PK: 4, Title: "Blockers?", Notes: [][2]string{{"Casey Diaz", "None"}}},
			}, nil),
		},
		recognitionFeed:  fakeFeedItems("r", 120),
		notificationFeed: fakeFeedItems("n", 120),
		requests:         map[string]int{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func writeFakeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		panic(err)
	}
}

func (f *fake7Geese) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	path := r.URL.Path
	key := path
	if path == "/graphql" {
		key = "graphql:" + r.URL.Query().Get("opname")
	}
	f.requests[key]++
	if remaining, ok := f.failures[key]; ok && remaining != 0 {
		if remaining > 0 {
			f.failures[key] = remaining - 1
		}
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	cookie, err := r.Cookie("sgsession4")
	loggedIn := err == nil && cookie.Value == fakeSession
	if path == "/status/" {
		status := map[string]any{"loggedIn": loggedIn}
		if loggedIn {
			status["userId"] = f.userID
		}
		writeFakeJSON(w, status)
		return
	}
	if !loggedIn {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	switch {
	case path == "/graphql":
		f.graphQL(w, r)
	case path == "/api/v1/feeds/recognition/":
		f.recognition(w, r)
	case path == "/api/v1/notifications/":
		f.notifications(w, r)
	default:
		if object, ok := f.single[path]; ok {
			writeFakeJSON(w, object)
			return
		}
		items, ok := f.lists[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		f.list(w, r, items)
	}
}

func (f *fake7Geese) list(w http.ResponseWriter, r *http.Request, items []map[string]any) {
	path := r.URL.Path
	query := r.URL.Query()
	limit, limitErr := strconv.Atoi(query.Get("limit"))
	offset, offsetErr := strconv.Atoi(query.Get("offset"))
	if limitErr != nil || offsetErr != nil || limit <= 0 || offset < 0 {
		http.Error(w, "bad paging", http.StatusBadRequest)
		return
	}
	if required, ok := f.requiredParams[path]; ok && query.Get(required[0]) != required[1] {
		http.Error(w, "missing filter "+required[0], http.StatusBadRequest)
		return
	}
	if f.ignoreOffset[path] {
		offset = 0
	}
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	var next any
	if end < len(items) {
		next = fmt.Sprintf("%s?limit=%d&offset=%d", path, limit, end)
	}
	objects := append([]map[string]any{}, items[offset:end]...)
	if f.unstableLists[path] && offset > 0 && len(objects) > 0 {
		objects[0] = items[offset-1]
	}
	writeFakeJSON(w, map[string]any{
		"meta": map[string]any{
			"limit": limit, "offset": offset, "previous": nil, "next": next,
			"total_count": len(items) + f.totalCountDelta[path],
		},
		"objects": objects,
	})
}

func (f *fake7Geese) recognition(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit <= 0 || query.Get("recipient") != profileURI(f.userID) {
		http.Error(w, "bad recognition feed request", http.StatusBadRequest)
		return
	}
	start := 0
	switch offset := query.Get("offset"); offset {
	case "":
	case "None":
		f.recognitionWraps++
	default:
		start, err = strconv.Atoi(offset)
		if err != nil {
			http.Error(w, "bad offset", http.StatusBadRequest)
			return
		}
	}
	end := start + limit
	if end > len(f.recognitionFeed) {
		end = len(f.recognitionFeed)
	}
	cursor := "None"
	if end < len(f.recognitionFeed) {
		cursor = strconv.Itoa(end)
	}
	next := fmt.Sprintf("%s?recipient=%s&limit=%d&offset=%s&skip=0", r.URL.Path, url.QueryEscape(profileURI(f.userID)), limit, cursor)
	writeFakeJSON(w, map[string]any{
		"meta":    map[string]any{"limit": limit, "next": next, "offset": "cursor-of-this-request"},
		"objects": append([]map[string]any{}, f.recognitionFeed[start:end]...),
	})
}

func (f *fake7Geese) notifications(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, err := strconv.Atoi(query.Get("limit"))
	if err != nil || limit <= 1 {
		http.Error(w, "bad notification feed request", http.StatusBadRequest)
		return
	}
	start := 0
	if offset := query.Get("offset"); offset != "" {
		start, err = strconv.Atoi(offset)
		if err != nil {
			http.Error(w, "bad offset", http.StatusBadRequest)
			return
		}
	}
	end := start + limit
	if end > len(f.notificationFeed) {
		end = len(f.notificationFeed)
	}
	var next any
	if end < len(f.notificationFeed) {
		next = fmt.Sprintf("%s?limit=%d&offset=%d&skip=1", r.URL.Path, limit, end-1)
	}
	writeFakeJSON(w, map[string]any{
		"meta":    map[string]any{"limit": limit, "next": next},
		"objects": append([]map[string]any{}, f.notificationFeed[start:end]...),
	})
}

func fakeCheckinsPage(objective fakeObjective, start, limit int) map[string]any {
	end := start + limit
	if end > len(objective.Checkins) {
		end = len(objective.Checkins)
	}
	edges := []any{}
	for _, checkin := range objective.Checkins[start:end] {
		edges = append(edges, map[string]any{"node": map[string]any{
			"pk": checkin.PK, "message": checkin.Message, "created": checkin.Created,
			"metadata": map[string]any{"metadata": map[string]any{
				"keyresult": []any{},
				"objective": map[string]any{"snapshot": map[string]any{"id": objective.PK, "name": objective.Name, "progress": checkin.Progress}},
			}},
			"user": fakeGQLUser(1, checkin.Author),
		}})
	}
	return map[string]any{
		"totalCount": len(objective.Checkins),
		"pageInfo":   map[string]any{"endCursor": strconv.Itoa(end), "hasNextPage": end < len(objective.Checkins)},
		"edges":      edges,
	}
}

func fakeObjectiveNode(objective fakeObjective, checkinLimit int) map[string]any {
	closedPage := map[string]any{"endCursor": "", "hasNextPage": false}
	var due any
	if objective.Due != "" {
		due = objective.Due
	}
	var lastCheckin any
	if len(objective.Checkins) > 0 {
		last := objective.Checkins[0]
		lastCheckin = map[string]any{"pk": last.PK, "message": last.Message, "created": last.Created, "user": fakeGQLUser(1, last.Author)}
	}
	return map[string]any{
		"pk": objective.PK, "name": objective.Name, "description": "", "closed": objective.Closed,
		"draft": false, "private": false, "progress": objective.Progress,
		"dueDatetime": due, "startDate": "2026-01-01", "completedDatetime": nil,
		"objectiveType": objective.Type, "currentUserParticipantType": objective.Role,
		"owners": map[string]any{"pageInfo": closedPage, "edges": []any{
			map[string]any{"node": map[string]any{"pk": 1, "fullName": "Alex Morgan", "position": "Engineer"}},
		}},
		"keyResults": map[string]any{"pageInfo": closedPage, "edges": []any{
			map[string]any{"node": map[string]any{
				"id": objective.PK*10 + 1, "name": "Key result", "description": "", "progress": 50.0,
				"currentValue": 5.0, "targetValue": 10.0, "startingValue": 0.0, "measurementType": 3,
			}},
		}},
		"checkins":    fakeCheckinsPage(objective, 0, checkinLimit),
		"lastCheckin": lastCheckin,
	}
}

func (f *fake7Geese) graphQL(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-CSRFToken") != fakeCSRF {
		http.Error(w, "csrf check failed", http.StatusForbidden)
		return
	}
	var request struct {
		OperationName string         `json:"operationName"`
		Variables     map[string]any `json:"variables"`
		Query         string         `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	intVar := func(name string) int {
		value, ok := request.Variables[name].(float64)
		if !ok {
			f.t.Errorf("GraphQL %s: variable %s is not a number: %v", request.OperationName, name, request.Variables[name])
		}
		return int(value)
	}
	cursorVar := func() int {
		after, ok := request.Variables["after"].(string)
		if !ok {
			f.t.Errorf("GraphQL %s: variable after is not a string", request.OperationName)
		}
		if after == "" {
			return 0
		}
		start, err := strconv.Atoi(after)
		if err != nil {
			f.t.Errorf("GraphQL %s: bad cursor %q", request.OperationName, after)
		}
		return start
	}

	var data any
	switch request.OperationName {
	case "getUserContextObjectives":
		filter := ""
		for _, name := range objectiveFilters {
			if strings.Contains(request.Query, name+": $userId") {
				filter = name
			}
		}
		if filter == "" || !strings.Contains(request.Query, "$userId: Int!") || !strings.Contains(request.Query, "closed,-due_datetime,pk") {
			writeFakeJSON(w, map[string]any{"errors": []any{map[string]any{"message": "bad objectives query"}}})
			return
		}
		objectives := f.objectives[intVar("userId")][filter]
		start := cursorVar()
		end := start + intVar("first")
		if end > len(objectives) {
			end = len(objectives)
		}
		edges := []any{}
		for _, objective := range objectives[start:end] {
			edges = append(edges, map[string]any{"node": fakeObjectiveNode(objective, intVar("checkins"))})
		}
		data = map[string]any{"objectives": map[string]any{
			"totalCount": len(objectives),
			"pageInfo":   map[string]any{"endCursor": strconv.Itoa(end), "hasNextPage": end < len(objectives)},
			"edges":      edges,
		}}
	case "getObjectiveCheckins":
		pk := intVar("pk")
		var found any
		for _, byFilter := range f.objectives {
			for _, objectives := range byFilter {
				for _, objective := range objectives {
					if objective.PK == pk {
						start := cursorVar()
						if f.stuckCheckins {
							start = 0
						}
						found = map[string]any{"pk": pk, "checkins": fakeCheckinsPage(objective, start, intVar("first"))}
					}
				}
			}
		}
		data = map[string]any{"objective": found}
	case "getReviewOverviewPastReviews":
		if _, filtered := request.Variables["completedState"]; filtered || strings.Contains(request.Query, "state:") {
			writeFakeJSON(w, map[string]any{"errors": []any{map[string]any{"message": "the review list must not have a state filter"}}})
			return
		}
		matching := f.reviews[intVar("profileId")]
		start := intVar("offset")
		if start > len(matching) {
			start = len(matching)
		}
		end := start + intVar("first")
		if end > len(matching) {
			end = len(matching)
		}
		edges := []any{}
		for _, review := range matching[start:end] {
			edges = append(edges, map[string]any{"node": map[string]any{
				"pk": review.Data["pk"], "startDate": review.Data["startDate"], "endDate": review.Data["endDate"],
				"snapshotGroup": map[string]any{"pk": 1, "title": "list title"},
			}})
		}
		data = map[string]any{"snapshots": map[string]any{"totalCount": len(matching), "edges": edges}}
	case "getSingleSnapshot":
		pk := intVar("snapshotId")
		var found any
		for _, reviews := range f.reviews {
			for _, review := range reviews {
				if review.Data["pk"] == pk {
					found = review.Data
				}
			}
		}
		data = map[string]any{"snapshot": found}
	case "listFeedbackRequests":
		if !strings.Contains(request.Query, "recipients: [$profileId]") || !strings.Contains(request.Query, "includeArchived: true") {
			writeFakeJSON(w, map[string]any{"errors": []any{map[string]any{"message": "bad feedback request list query"}}})
			return
		}
		var matching []fakeFeedbackEntry
		for _, entry := range f.feedback {
			if entry.Listed && entry.Recipient == intVar("profileId") {
				matching = append(matching, entry)
			}
		}
		start := intVar("offset")
		if start > len(matching) {
			start = len(matching)
		}
		end := start + intVar("first")
		if end > len(matching) {
			end = len(matching)
		}
		edges := []any{}
		for _, entry := range matching[start:end] {
			edges = append(edges, map[string]any{"node": map[string]any{"pk": entry.Data["pk"], "recipient": entry.Data["recipient"]}})
		}
		data = map[string]any{"feedbackRequests": map[string]any{"totalCount": len(matching), "edges": edges}}
	case "getFeedbackRequest":
		pk := intVar("pk")
		var found any
		for _, entry := range f.feedback {
			if entry.Data["pk"] == pk && !entry.Hidden {
				found = entry.Data
			}
		}
		data = map[string]any{"feedbackRequest": found}
	case "getFinalizedOneonone":
		var found any
		if meeting, ok := f.meetings[intVar("oneononeId")]; ok {
			found = meeting
		}
		data = map[string]any{"oneonone": found}
	default:
		http.Error(w, "unknown operation "+request.OperationName, http.StatusBadRequest)
		return
	}
	writeFakeJSON(w, map[string]any{"data": data})
}

func (f *fake7Geese) change(mutate func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mutate()
}

func (f *fake7Geese) requestCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[key]
}

func (f *fake7Geese) dataRequestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	total := 0
	for key, count := range f.requests {
		if key != "/status/" {
			total += count
		}
	}
	return total
}

type recordingReporter struct {
	mu       sync.Mutex
	done     map[string]int
	failed   map[string]string
	warnings map[string][]string
	purged   []string
}

func newRecordingReporter() *recordingReporter {
	return &recordingReporter{done: map[string]int{}, failed: map[string]string{}, warnings: map[string][]string{}}
}

func (r *recordingReporter) Start(key store.DatasetKey) {}

func (r *recordingReporter) Done(key store.DatasetKey, count int, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done[key.String()] = count
}

func (r *recordingReporter) Failed(key store.DatasetKey, err error, elapsed time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed[key.String()] = err.Error()
}

func (r *recordingReporter) Warning(key store.DatasetKey, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.warnings[key.String()] = append(r.warnings[key.String()], message)
}

func (r *recordingReporter) Purged(key store.DatasetKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.purged = append(r.purged, key.String())
}

func (r *recordingReporter) Summary(records, succeeded, failed int, elapsed time.Duration) {}

type syncFixture struct {
	fake       *fake7Geese
	db         *store.Store
	dbPath     string
	configPath string
	reporter   *recordingReporter
}

func writeFakeConfig(t *testing.T, path, baseURL, session string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"base_url": baseURL, "session": session, "csrf": fakeCSRF})
	if err != nil {
		t.Fatalf("encoding config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
}

func isolateSyncEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("API_7GEESE_BASE_URL", "")
	t.Setenv("API_7GEESE_CONFIG", "")
	t.Setenv("SEVENGEESE_SESSION", "")
	t.Setenv("SEVENGEESE_CSRF", "")
	previousRefresh, previousLogin, previousDelay := refreshSession, browserLogin, syncRetryBaseDelay
	previousPageSizes := syncListPageSizes
	previousTerminal, previousStdin := stdinIsTerminal, sharedStdin
	refreshSession = func(string) bool { return false }
	browserLogin = func() (string, string, error) { return "", "", errors.New("no browser in tests") }
	syncRetryBaseDelay = time.Millisecond
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() {
		refreshSession, browserLogin, syncRetryBaseDelay = previousRefresh, previousLogin, previousDelay
		syncListPageSizes = previousPageSizes
		stdinIsTerminal, sharedStdin = previousTerminal, previousStdin
	})
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	isolateSyncEnvironment(t)
	fake := newFake7Geese(t)
	configPath := filepath.Join(t.TempDir(), "config.json")
	writeFakeConfig(t, configPath, fake.server.URL, fakeSession)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &syncFixture{fake: fake, db: db, dbPath: dbPath, configPath: configPath, reporter: newRecordingReporter()}
}

func (x *syncFixture) options() syncOptions {
	return syncOptions{ConfigPath: x.configPath, Timeout: 5 * time.Second, Reporter: x.reporter}
}

func (x *syncFixture) sync() error {
	return runSync(context.Background(), x.db, x.options())
}

func (x *syncFixture) mustSync(t *testing.T) {
	t.Helper()
	if err := x.sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (x *syncFixture) ids(t *testing.T, key store.DatasetKey) []string {
	t.Helper()
	ids, err := x.db.RecordIDs(key)
	if err != nil {
		t.Fatalf("listing %s: %v", key, err)
	}
	return ids
}

func (x *syncFixture) datasetNames(t *testing.T) []string {
	t.Helper()
	keys, err := x.db.Datasets()
	if err != nil {
		t.Fatalf("listing datasets: %v", err)
	}
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	return names
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
