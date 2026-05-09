// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0. See LICENSE.
// Copyright 2026 ekin-inceleme. Licensed under Apache-2.0.

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"7geese-cli/internal/cli"
	"7geese-cli/internal/cliutil"
	"7geese-cli/internal/client"
	"7geese-cli/internal/config"
	"7geese-cli/internal/mcp/cobratree"
	"7geese-cli/internal/store"
)

// RegisterTools registers all API operations as MCP tools.
func RegisterTools(s *server.MCPServer) {
	s.AddTool(
		mcplib.NewTool("badges_list",
			mcplib.WithDescription(""),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/badges/", []mcpParamBinding{ }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("categories_list",
			mcplib.WithDescription(""),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/categories/", []mcpParamBinding{ }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("checkins_create",
			mcplib.WithDescription("Required: message."),
			mcplib.WithString("message", mcplib.Required(), mcplib.Description("Check-in message/status update")),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("POST", "/api/v1/checkins/", []mcpParamBinding{{PublicName: "message", WireName: "message", Location: "body"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("checkins_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/checkins/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("checkins_list",
			mcplib.WithDescription("Optional: limit, offset, creator."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithString("creator", mcplib.Description("Filter by creator user ID")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/checkins/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "creator", WireName: "creator", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("feedbackrequest_create",
			mcplib.WithDescription("Optional: provider, message."),
			mcplib.WithString("provider", mcplib.Description("User URI of feedback provider")),
			mcplib.WithString("message", mcplib.Description("")),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("POST", "/api/v1/feedbackrequest/", []mcpParamBinding{{PublicName: "provider", WireName: "provider", Location: "body"},{PublicName: "message", WireName: "message", Location: "body"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("feedbackrequest_list",
			mcplib.WithDescription("Optional: limit."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/feedbackrequest/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("notifications_list",
			mcplib.WithDescription("Optional: limit, offset."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/notifications/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("objectivekeyresults_create",
			mcplib.WithDescription("Required: name, objective. Optional: measurement_type, starting_value, target_value."),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("")),
			mcplib.WithString("objective", mcplib.Required(), mcplib.Description("Resource URI of parent objective")),
			mcplib.WithString("measurement_type", mcplib.Description("")),
			mcplib.WithString("starting_value", mcplib.Description("")),
			mcplib.WithString("target_value", mcplib.Description("")),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("POST", "/api/v1/objectivekeyresults/", []mcpParamBinding{{PublicName: "name", WireName: "name", Location: "body"},{PublicName: "objective", WireName: "objective", Location: "body"},{PublicName: "measurement_type", WireName: "measurement_type", Location: "body"},{PublicName: "starting_value", WireName: "starting_value", Location: "body"},{PublicName: "target_value", WireName: "target_value", Location: "body"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("objectivekeyresults_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/objectivekeyresults/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("objectivekeyresults_list",
			mcplib.WithDescription("Optional: objective, limit, offset."),
			mcplib.WithString("objective", mcplib.Description("Filter by objective resource URI")),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/objectivekeyresults/", []mcpParamBinding{{PublicName: "objective", WireName: "objective", Location: "query"},{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("objectivekeyresults_update",
			mcplib.WithDescription("Required: id. Optional: current_value, name. Partial update."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithString("current_value", mcplib.Description("Update progress on this key result")),
			mcplib.WithString("name", mcplib.Description("")),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("PATCH", "/api/v1/objectivekeyresults/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"},{PublicName: "current_value", WireName: "current_value", Location: "body"},{PublicName: "name", WireName: "name", Location: "body"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("objectives_create",
			mcplib.WithDescription("Required: name. Optional: description, objective_type, due_date (plus 4 more)."),
			mcplib.WithString("name", mcplib.Required(), mcplib.Description("")),
			mcplib.WithString("description", mcplib.Description("")),
			mcplib.WithString("objective_type", mcplib.Description("personal, team, or org")),
			mcplib.WithString("due_date", mcplib.Description("ISO 8601 date")),
			mcplib.WithString("start_date", mcplib.Description("")),
			mcplib.WithString("measurement_type", mcplib.Description("percent, number, boolean")),
			mcplib.WithString("starting_value", mcplib.Description("")),
			mcplib.WithString("target_value", mcplib.Description("")),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("POST", "/api/v1/objectives/", []mcpParamBinding{{PublicName: "name", WireName: "name", Location: "body"},{PublicName: "description", WireName: "description", Location: "body"},{PublicName: "objective_type", WireName: "objective_type", Location: "body"},{PublicName: "due_date", WireName: "due_date", Location: "body"},{PublicName: "start_date", WireName: "start_date", Location: "body"},{PublicName: "measurement_type", WireName: "measurement_type", Location: "body"},{PublicName: "starting_value", WireName: "starting_value", Location: "body"},{PublicName: "target_value", WireName: "target_value", Location: "body"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("objectives_delete",
			mcplib.WithDescription("Required: id. Destructive."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithDestructiveHintAnnotation(true),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("DELETE", "/api/v1/objectives/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("objectives_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/objectives/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("objectives_list",
			mcplib.WithDescription("Optional: limit, offset, objective_type (plus 1 more)."),
			mcplib.WithNumber("limit", mcplib.Description("Number of results (default 20)")),
			mcplib.WithNumber("offset", mcplib.Description("Pagination offset")),
			mcplib.WithString("objective_type", mcplib.Description("Filter by type: personal, team, org")),
			mcplib.WithBoolean("closed", mcplib.Description("Filter closed objectives")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/objectives/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "objective_type", WireName: "objective_type", Location: "query"},{PublicName: "closed", WireName: "closed", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("objectives_update",
			mcplib.WithDescription("Required: id. Optional: name, description, progress (plus 1 more). Partial update."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithString("name", mcplib.Description("")),
			mcplib.WithString("description", mcplib.Description("")),
			mcplib.WithString("progress", mcplib.Description("Progress value (0-100 for percent type)")),
			mcplib.WithBoolean("closed", mcplib.Description("")),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("PATCH", "/api/v1/objectives/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"},{PublicName: "name", WireName: "name", Location: "body"},{PublicName: "description", WireName: "description", Location: "body"},{PublicName: "progress", WireName: "progress", Location: "body"},{PublicName: "closed", WireName: "closed", Location: "body"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("oneononenotes_list",
			mcplib.WithDescription("Optional: oneonone, limit."),
			mcplib.WithString("oneonone", mcplib.Description("Filter by 1:1 ID")),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/oneononenotes/", []mcpParamBinding{{PublicName: "oneonone", WireName: "oneonone", Location: "query"},{PublicName: "limit", WireName: "limit", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("oneonones_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/oneonones/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("oneonones_list",
			mcplib.WithDescription("Optional: limit, offset, status (plus 1 more)."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithString("status", mcplib.Description("Filter by status: upcoming, completed")),
			mcplib.WithString("target", mcplib.Description("Filter by target user")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/oneonones/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "status", WireName: "status", Location: "query"},{PublicName: "target", WireName: "target", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("organizationalobjectives_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/organizationalobjectives/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("organizationalobjectives_list",
			mcplib.WithDescription("Optional: limit, offset."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/organizationalobjectives/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("peer_feedback_list",
			mcplib.WithDescription("Optional: limit, offset."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/feedback/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("performancecycles_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/performancecycles/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("performancecycles_list",
			mcplib.WithDescription("Optional: limit, target."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithString("target", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/performancecycles/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "target", WireName: "target", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("recognitionbadges_create",
			mcplib.WithDescription("Required: recipient, badge, message. Optional: quiet."),
			mcplib.WithString("recipient", mcplib.Required(), mcplib.Description("User URI of recipient")),
			mcplib.WithString("badge", mcplib.Required(), mcplib.Description("Badge URI")),
			mcplib.WithString("message", mcplib.Required(), mcplib.Description("")),
			mcplib.WithBoolean("quiet", mcplib.Description("Suppress network post")),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("POST", "/api/v1/recognitionbadges/", []mcpParamBinding{{PublicName: "recipient", WireName: "recipient", Location: "body"},{PublicName: "badge", WireName: "badge", Location: "body"},{PublicName: "message", WireName: "message", Location: "body"},{PublicName: "quiet", WireName: "quiet", Location: "body"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("recognitionbadges_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/recognitionbadges/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("recognitionbadges_list",
			mcplib.WithDescription("Optional: limit, offset, sender (plus 1 more)."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithString("sender", mcplib.Description("")),
			mcplib.WithString("recipient", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/recognitionbadges/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "sender", WireName: "sender", Location: "query"},{PublicName: "recipient", WireName: "recipient", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("team_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/team/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("team_list",
			mcplib.WithDescription("Optional: limit."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/team/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("teamobjectives_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/teamobjectives/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("teamobjectives_list",
			mcplib.WithDescription("Optional: limit, offset, closed."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithBoolean("closed", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/teamobjectives/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "closed", WireName: "closed", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("user_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/user/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("user_list",
			mcplib.WithDescription("Optional: limit, offset, email (plus 1 more)."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithNumber("offset", mcplib.Description("")),
			mcplib.WithString("email", mcplib.Description("")),
			mcplib.WithBoolean("is_active", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/user/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"},{PublicName: "offset", WireName: "offset", Location: "query"},{PublicName: "email", WireName: "email", Location: "query"},{PublicName: "is_active", WireName: "is_active", Location: "query"}, }, []string{ }),
	)
	s.AddTool(
		mcplib.NewTool("userprofile_get",
			mcplib.WithDescription("Required: id."),
			mcplib.WithString("id", mcplib.Required(), mcplib.Description("id")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/userprofile/{id}/", []mcpParamBinding{{PublicName: "id", WireName: "id", Location: "path"}, }, []string{"id", }),
	)
	s.AddTool(
		mcplib.NewTool("userprofile_list",
			mcplib.WithDescription("Optional: limit."),
			mcplib.WithNumber("limit", mcplib.Description("")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
			mcplib.WithOpenWorldHintAnnotation(true),
		),
		makeAPIHandler("GET", "/api/v1/userprofile/", []mcpParamBinding{{PublicName: "limit", WireName: "limit", Location: "query"}, }, []string{ }),
	)
	// Search tool — faster than iterating list endpoints for finding specific items
	s.AddTool(
		mcplib.NewTool("search",
			mcplib.WithDescription("Full-text search across all synced data. Faster than paginating list endpoints. Requires sync first."),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("Search query (supports FTS5 syntax: AND, OR, NOT, quotes for phrases)")),
			mcplib.WithNumber("limit", mcplib.Description("Max results (default 25)")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
		),
		handleSearch,
	)
	// SQL tool — ad-hoc analysis on synced data without API calls
	s.AddTool(
		mcplib.NewTool("sql",
			mcplib.WithDescription("Run read-only SQL against local database. Use for ad-hoc analysis, aggregations, and joins across synced resources. Requires sync first."),
			mcplib.WithString("query", mcplib.Required(), mcplib.Description("SQL query (SELECT or WITH...SELECT). Tables match resource names.")),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
		),
		handleSQL,
	)

	// Context tool — front-loaded domain knowledge for agents.
	// Call this first to understand the API taxonomy, query patterns, and capabilities.
	s.AddTool(
		mcplib.NewTool("context",
			mcplib.WithDescription("Get API domain context: resource taxonomy, auth requirements, query tips, and unique capabilities. Call this first."),
			mcplib.WithReadOnlyHintAnnotation(true),
			mcplib.WithDestructiveHintAnnotation(false),
		),
		handleContext,
	)

	// Runtime Cobra-tree mirror — exposes every user-facing command that is
	// not already covered by a typed endpoint or framework MCP tool.
	cobratree.RegisterAll(s, cli.RootCmd(), cobratree.SiblingCLIPath)
}

type mcpParamBinding struct {
	PublicName string
	WireName   string
	Location   string
}

// makeAPIHandler creates a generic MCP tool handler for an API endpoint.
func makeAPIHandler(method, pathTemplate string, bindings []mcpParamBinding, positionalParams []string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		c, err := newMCPClient()
		if err != nil {
			return mcplib.NewToolResultError(err.Error()), nil
		}

		// mcp-go v0.47+ made CallToolParams.Arguments an `any` to support
		// non-map payloads; GetArguments() returns the map[string]any shape
		// we rely on here (or an empty map when the payload is something else).
		args := req.GetArguments()

		// positionalParams mixes real URL path params with CLI positional
		// args that map to query params (e.g. `search <query>` -> ?query=);
		// the placeholder check below disambiguates them at runtime.
		path := pathTemplate
		knownArgs := make(map[string]bool, len(bindings))
		pathParams := make(map[string]bool, len(positionalParams))
		params := make(map[string]string)
		bodyArgs := make(map[string]any)
		for _, binding := range bindings {
			knownArgs[binding.PublicName] = true
			v, ok := args[binding.PublicName]
			if !ok {
				continue
			}
			switch binding.Location {
			case "path":
				placeholder := "{" + binding.WireName + "}"
				pathParams[binding.PublicName] = true
				path = strings.Replace(path, placeholder, fmt.Sprintf("%v", v), 1)
			case "body":
				bodyArgs[binding.WireName] = v
			default:
				params[binding.WireName] = fmt.Sprintf("%v", v)
			}
		}
		for _, p := range positionalParams {
			placeholder := "{" + p + "}"
			if !strings.Contains(pathTemplate, placeholder) {
				continue
			}
			pathParams[p] = true
			if v, ok := args[p]; ok {
				path = strings.Replace(path, placeholder, fmt.Sprintf("%v", v), 1)
			}
		}

		for k, v := range args {
			if pathParams[k] || knownArgs[k] {
				continue
			}
			switch method {
			case "POST", "PUT", "PATCH":
				bodyArgs[k] = v
			default:
				params[k] = fmt.Sprintf("%v", v)
			}
		}

		var data json.RawMessage
		switch method {
		case "GET":
			data, err = c.Get(path, params)
		case "POST":
			body, _ := json.Marshal(bodyArgs)
			data, _, err = c.Post(path, body)
		case "PUT":
			body, _ := json.Marshal(bodyArgs)
			data, _, err = c.Put(path, body)
		case "PATCH":
			body, _ := json.Marshal(bodyArgs)
			data, _, err = c.Patch(path, body)
		case "DELETE":
			data, _, err = c.Delete(path)
		default:
			return mcplib.NewToolResultError("unsupported method: " + method), nil
		}

		if err != nil {
			msg := err.Error()
			switch {
			case strings.Contains(msg, "HTTP 409"):
				return mcplib.NewToolResultText("already exists (no-op)"), nil
			case strings.Contains(msg, "HTTP 400") && cliutil.LooksLikeAuthError(msg):
				return mcplib.NewToolResultError("authentication error: " + cliutil.SanitizeErrorBody(msg) +
					"\nhint: the API rejected the request — this usually means auth is missing or invalid." +
					"\n      Set your API key: export SEVENGEESE_SESSION=<your-key>" +
					"\n      Run '7geese-cli doctor' to check auth status."), nil
			case strings.Contains(msg, "HTTP 401"):
				return mcplib.NewToolResultError("authentication failed: " + cliutil.SanitizeErrorBody(msg) +
					"\nhint: check your API credentials." +
					"\n      Set it with: export SEVENGEESE_SESSION=<your-key>" +
					"\n      Run '7geese-cli doctor' to check auth status."), nil
			case strings.Contains(msg, "HTTP 403"):
				return mcplib.NewToolResultError("permission denied: " + cliutil.SanitizeErrorBody(msg) +
					"\nhint: your credentials are valid but lack access to this resource." +
					"\n      Set it with: export SEVENGEESE_SESSION=<your-key>" +
					"\n      Run '7geese-cli doctor' to check auth status."), nil
			case strings.Contains(msg, "HTTP 404"):
				if method == "DELETE" {
					return mcplib.NewToolResultText("already deleted (no-op)"), nil
				}
				return mcplib.NewToolResultError("not found: " + msg), nil
			case strings.Contains(msg, "HTTP 429"):
				return mcplib.NewToolResultError("rate limited: " + msg), nil
			default:
				return mcplib.NewToolResultError(msg), nil
			}
		}

		// For GET responses, wrap bare arrays with count metadata
		if method == "GET" {
			trimmed := strings.TrimSpace(string(data))
			if len(trimmed) > 0 && trimmed[0] == '[' {
				var items []json.RawMessage
				if json.Unmarshal(data, &items) == nil {
					wrapped := map[string]any{
						"count": len(items),
						"items": items,
					}
					out, _ := json.Marshal(wrapped)
					return mcplib.NewToolResultText(string(out)), nil
				}
			}
		}
		return mcplib.NewToolResultText(string(data)), nil
	}
}

func newMCPClient() (*client.Client, error) {
	home, _ := os.UserHomeDir()
	cfgPath := filepath.Join(home, ".config", "7geese-cli", "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	c := client.New(cfg, 30*time.Second, 0)
	// Agents calling through MCP need fresh data every call. The on-disk
	// response cache survives across MCP server invocations, so a
	// DELETE/PATCH followed by a GET would otherwise return the
	// pre-mutation snapshot for up to the cache TTL. The interactive CLI
	// constructs its own client and is unaffected.
	c.NoCache = true
	return c, nil
}

func dbPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "7geese-cli", "data.db")
}
// Note: MCP tools use their own dbPath() because they are in a separate package (main, not cli).
// The CLI's defaultDBPath() in the cli package uses the same canonical path.

func handleSearch(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcplib.NewToolResultError("query is required"), nil
	}

	limit := 25
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	db, err := store.OpenReadOnly(dbPath())
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("opening database: %v", err)), nil
	}
	defer db.Close()

	results, err := db.Search(query, limit)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("search failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

// validateReadOnlyQuery gates the MCP sql tool. The agent contract advertised
// to the host is ReadOnlyHintAnnotation(true); a false annotation on a
// mutating tool lets MCP hosts auto-approve writes and is treated as a real
// bug per the project's agent-native security model.
//
// The gate is an allowlist (SELECT or WITH only) applied AFTER stripping the
// leading whitespace, line comments, block comments, and semicolons that
// SQLite itself ignores before parsing. A naive HasPrefix check on a
// keyword blocklist is bypassable by prefixing the dangerous statement with
// "/* x */" or "-- x\n" — TrimSpace strips outer whitespace but does not
// understand SQL comment syntax. Combined with the empirical fact that
// modernc.org/sqlite's mode=ro does NOT block VACUUM INTO (writes a snapshot
// to a new file) or ATTACH DATABASE (opens a separate writable handle),
// such a bypass produces silent exfiltration to an attacker-chosen path.
//
// SELECT and WITH are the only allowed leading keywords. WITH supports
// SELECT-form CTEs; CTE-wrapped writes ("WITH x AS (...) INSERT ...") are
// caught by OpenReadOnly's mode=ro one layer down. PRAGMA, ATTACH, VACUUM,
// and every other DDL/DML keyword fail at this gate before reaching SQLite.
func validateReadOnlyQuery(query string) error {
	upper := strings.ToUpper(stripLeadingSQLNoise(query))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return fmt.Errorf("only SELECT queries are allowed")
	}
	return nil
}

// stripLeadingSQLNoise removes leading whitespace, SQL line comments
// (-- to end of line), block comments (/* ... */), and statement
// separators (;) from query. SQLite skips these before parsing the first
// keyword, so a security gate that does not strip them mismatches what the
// driver actually executes.
func stripLeadingSQLNoise(query string) string {
	for {
		query = strings.TrimLeft(query, " \t\r\n;")
		switch {
		case strings.HasPrefix(query, "--"):
			if idx := strings.IndexByte(query, '\n'); idx >= 0 {
				query = query[idx+1:]
				continue
			}
			return ""
		case strings.HasPrefix(query, "/*"):
			if idx := strings.Index(query[2:], "*/"); idx >= 0 {
				query = query[2+idx+2:]
				continue
			}
			return ""
		default:
			return query
		}
	}
}

func handleSQL(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	args := req.GetArguments()
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcplib.NewToolResultError("query is required"), nil
	}

	if err := validateReadOnlyQuery(query); err != nil {
		return mcplib.NewToolResultError(err.Error()), nil
	}

	db, err := store.OpenReadOnly(dbPath())
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("opening database: %v", err)), nil
	}
	defer db.Close()

	rows, err := db.Query(query)
	if err != nil {
		return mcplib.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		rows.Scan(ptrs...)
		row := make(map[string]any)
		for i, col := range cols {
			row[col] = values[i]
		}
		results = append(results, row)
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

func handleContext(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
	ctx := map[string]any{
		"api":         "7geese",
		"description": "The first CLI for 7Geese — log in with Chrome, query OKRs, check-ins, and 1:1s from the terminal.",
		"archetype":   "generic",
		"tool_count":  36,
		// tool_surface tells agents which surface a capability lives on.
		"tool_surface": "MCP exposes typed endpoint tools plus a runtime mirror of user-facing CLI commands. Endpoint tools keep typed schemas; command-mirror tools shell out to the companion 7geese-cli binary.",
		"auth": map[string]any{
			"type": "cookie",
			"env_vars": []map[string]any{
				{
					"name": "SEVENGEESE_SESSION",
					"kind": "per_call",
					"required": true,
					"sensitive": true,
					"description": "Set to your API credential.",
				},
			},
		},
		"resources": []map[string]any{
			{
				"name": "badges",
				"description": "Available badge types for recognition",
				"endpoints": []string{"list",  },
				"syncable": true,
			},
			{
				"name": "categories",
				"description": "Objective categories for tagging",
				"endpoints": []string{"list",  },
				"syncable": true,
			},
			{
				"name": "checkins",
				"description": "Weekly check-ins on goals and progress",
				"endpoints": []string{"create", "get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "feedbackrequest",
				"description": "Feedback requests sent to peers",
				"endpoints": []string{"create", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "notifications",
				"description": "User notifications",
				"endpoints": []string{"list",  },
				"syncable": true,
			},
			{
				"name": "objectivekeyresults",
				"description": "Key results belonging to objectives",
				"endpoints": []string{"create", "get", "list", "update",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "objectives",
				"description": "Personal OKRs and goals",
				"endpoints": []string{"create", "delete", "get", "list", "update",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "oneononenotes",
				"description": "Notes attached to one-on-one meetings",
				"endpoints": []string{"list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "oneonones",
				"description": "One-on-one meetings between manager and report",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "organizationalobjectives",
				"description": "Company-wide OKRs",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "peer_feedback",
				"description": "Peer feedback requests and responses",
				"endpoints": []string{"list",  },
				"syncable": true,
			},
			{
				"name": "performancecycles",
				"description": "Performance review cycles",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "recognitionbadges",
				"description": "Recognition and kudos sent between users",
				"endpoints": []string{"create", "get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "team",
				"description": "Teams in the organization",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "teamobjectives",
				"description": "Team-level OKRs",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "user",
				"description": "Users in the organization",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
			{
				"name": "userprofile",
				"description": "Extended user profile with role and manager info",
				"endpoints": []string{"get", "list",  },
				"syncable": true,
				"searchable": true,
			},
		},
		"query_tips": []string{
			"Pagination uses cursor-based paging. Pass after parameter for subsequent pages.",
			"Control page size with the limit parameter (default 100).",
			"Use the sql tool for ad-hoc analysis on synced data. Run sync first to populate the local database.",
			"Use the search tool for full-text search across all synced resources. Faster than iterating list endpoints.",
			"Prefer sql/search over repeated API calls when the data is already synced.",
		},
		// Command-mirror capabilities are exposed through MCP by shelling out
		// to the companion CLI binary.
		"command_mirror_capabilities": []map[string]string{
			{"name": "Browser Cookie Auth", "command": "auth login --chrome", "description": "Log in by reading your existing Chrome (or Firefox/Safari) session — no API key needed.", "rationale": "7Geese has no API token endpoint; Okta SSO is the only auth path. sweetcookie reads the encrypted browser cookie...", "via": "mcp-command-mirror"},
			{"name": "OKR Health Dashboard", "command": "okr health", "description": "See which objectives are on track, at risk, or stale — across personal, team, and org levels.", "rationale": "Requires joining objectives, key results, and due dates in SQLite; no single API endpoint returns this cross-level view.", "via": "mcp-command-mirror"},
			{"name": "Stale Objective Detector", "command": "objectives stale", "description": "Find objectives that have not been updated in N days.", "rationale": "Requires time-windowed query across modified timestamps in SQLite; not available via any API filter.", "via": "mcp-command-mirror"},
			{"name": "Check-in Streak Tracker", "command": "checkins streak", "description": "See consecutive weekly check-in streaks for yourself or your team.", "rationale": "Requires time-series aggregation across the full check-in history in SQLite; 98k records make this impractical via...", "via": "mcp-command-mirror"},
			{"name": "Manager Dashboard", "command": "manager dashboard", "description": "Pre-1:1 brief: direct reports, their recent check-ins, OKR health, and upcoming meetings in one view.", "rationale": "Requires joining users, check-ins, objectives, and oneonones — a cross-entity summary no single UI page provides.", "via": "mcp-command-mirror"},
			{"name": "Recognition Leaderboard", "command": "recognize leaderboard", "description": "See who gives and receives the most recognition this month.", "rationale": "Requires SQLite aggregation across 25k recognition badge records by period; not available in the 7Geese UI.", "via": "mcp-command-mirror"},
			{"name": "My Week Summary", "command": "me week", "description": "Everything relevant to you this week: check-ins due, OKRs to update, upcoming 1:1s.", "rationale": "Cross-entity join of check-ins, objectives, and oneonones for the past/next 7 days.", "via": "mcp-command-mirror"},
		},
		"playbook": []map[string]string{
			{"topic": "Browser Cookie Auth", "insight": "7Geese has no API token endpoint; Okta SSO is the only auth path. sweetcookie reads the encrypted browser cookie store cross-platform."},
			{"topic": "OKR Health Dashboard", "insight": "Requires joining objectives, key results, and due dates in SQLite; no single API endpoint returns this cross-level view."},
			{"topic": "Stale Objective Detector", "insight": "Requires time-windowed query across modified timestamps in SQLite; not available via any API filter."},
			{"topic": "Check-in Streak Tracker", "insight": "Requires time-series aggregation across the full check-in history in SQLite; 98k records make this impractical via paginated API calls."},
			{"topic": "Manager Dashboard", "insight": "Requires joining users, check-ins, objectives, and oneonones — a cross-entity summary no single UI page provides."},
			{"topic": "Recognition Leaderboard", "insight": "Requires SQLite aggregation across 25k recognition badge records by period; not available in the 7Geese UI."},
			{"topic": "My Week Summary", "insight": "Cross-entity join of check-ins, objectives, and oneonones for the past/next 7 days."},
		},
	}
	data, _ := json.MarshalIndent(ctx, "", "  ")
	return mcplib.NewToolResultText(string(data)), nil
}

// RegisterNovelFeatureTools is kept as a compatibility no-op for older MCP
// mains. New generated mains call RegisterTools only; RegisterTools now
// includes the runtime Cobra-tree mirror.
func RegisterNovelFeatureTools(s *server.MCPServer) {
	_ = s
}
