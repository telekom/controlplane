package security

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/security/mock"
	"github.com/telekom/controlplane/common-server/pkg/server/middleware/util"
)

var handlerMock = func(c *fiber.Ctx) error {
	bCtx, ok := util.NotNilOfType[*BusinessContext](c.Locals("businessContext"))
	if !ok {
		return c.SendString("Invalid authorization context")
	}
	prefix, ok := c.Locals("prefix").(string)
	if !ok {
		prefix = ""
	}
	return c.SendString("prefix:" + prefix + ", group:" + bCtx.Group + ", team:" + bCtx.Team)
}

type TestEnvironment struct {
	TestCases       []TestCase
	Description     string
	PathParameters  string
	RequestResource string
	Templates       map[ClientType]ComparisonTemplates
}

type TestCase struct {
	Method         string
	AccessToken    string
	ResourcePath   string
	ExpectedStatus int
	ExpectedBody   string
	Description    string // Added description field
}

var env = "test"

func TestCheckAccess(t *testing.T) {
	var testEnvironments = []TestEnvironment{
		{
			Description:     "Team-Resource in <env>--<group>--<team>/<resourceName> (default)",
			PathParameters:  ":namespace/:name",
			RequestResource: "/foo",
			TestCases: []TestCase{
				// Team Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "--group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team/, group:group, team:team",
					Description:    "Valid token with group and team, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "--group",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid Token with group and team, namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "", nil),
					ResourcePath:   env + "--group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token (no team), namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "", "", nil),
					ResourcePath:   env + "--group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token (no group and no team), namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("", "", "", nil),
					ResourcePath:   env + "--group",
					ExpectedStatus: 403,
					ExpectedBody:   "Missing field 'env'",
					Description:    "Token missing 'env' field",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "--group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team/, group:group, team:team",
					Description:    "Valid token with obfuscated team scope, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "--othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "--group2",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (extended group name, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "--group--teamplayer",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (extended team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "--group--te",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (shortened team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "--group--foo",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different team)",
				},
				// Group Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--group--foo",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace (different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--group2--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (extended group name, same team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--gr--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (shortened group name, same team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--foo--bar",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (different group, different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "--group2",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (extended group name, no team)",
				},
				// Admin Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "--othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:all"}),
					ResourcePath:   env + "--othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin all scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "--othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "POST-request with valid token with admin read scope, namespace mismatch (different group)",
				},

				// Global

				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team/, group:group, team:team",
					Description:    "Valid token with group and team, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--, group:group, team:team",
					Description:    "Valid token with group scope, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:all"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team/, group:group, team:team",
					Description:    "[POST] Valid token with team all scope, no namespace",
				},
			},
		},
		{
			Description:     "Environment-Team <env>/<group>--<team>",
			PathParameters:  ":namespace/:name",
			RequestResource: "", // no additional resource needed
			Templates: map[ClientType]ComparisonTemplates{
				ClientTypeTeam: {
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					MatchType:         MatchTypeEqual,
				},
				ClientTypeGroup: {
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					MatchType:         MatchTypePrefix,
				},
				ClientTypeAdmin: {
					ExpectedTemplate:  "{{ .B.Environment }}/",
					UserInputTemplate: "{{ .P.Namespace }}/",
					MatchType:         MatchTypePrefix,
				},
			},
			TestCases: []TestCase{
				// Team Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with group and team, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "", nil),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token with group but no team, namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "", "", nil),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token with no group and no team, namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with obfuscated team scope, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--teamplayer",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (extended team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--te",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (shortened team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--foo",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "/othergroup--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group)",
				},
				// Group Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group--foo",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace (different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/othergroup--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/gr--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (shortened group name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group2--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (extended group name)",
				},
				// Admin Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:all"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "[POST] Valid token with admin all scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with admin read scope, but post request",
				},

				// Global

				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with group and team, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--, group:group, team:team",
					Description:    "Valid token with group scope, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:all"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "[POST] Valid token with team all scope, no namespace",
				},
			},
		},
		{
			Description:     "Environment-Group <env>/<group>",
			PathParameters:  ":namespace/:name",
			RequestResource: "",
			Templates: map[ClientType]ComparisonTemplates{
				"group": {
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					MatchType:         MatchTypeEqual,
				},
				"admin": {
					ExpectedTemplate:  "{{ .B.Environment }}/",
					UserInputTemplate: "{{ .P.Namespace }}/",
					MatchType:         MatchTypePrefix,
				},
			},
			TestCases: []TestCase{
				// Team Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and team scope, matching namespace, but team scope not allowed on group",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/group2",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and team scope, matching namespace (extended group name), but team scope not allowed on group",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/gr",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and team scope, matching namespace (shortened group name), but team scope not allowed on group",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/foo",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and team scope, matching namespace (different group), but team scope not allowed on group",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, matching namespace group prefix, but team scope not allowed on group",
				},
				// Group Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group2",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and group scope, matching namespace (extended group name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/gr",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and group scope, matching namespace (shortened group name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/foo",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and group scope, matching namespace (different group)",
				},
				// Admin Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:all"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin all scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with admin read scope",
				},

				// Global

				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group and team and team scope, no namespace, team scope not supported",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group, group:group, team:team",
					Description:    "Valid token with group scope, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "InValid token with group, no team, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:all"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team all scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:all"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group, group:group, team:team",
					Description:    "[POST] Valid token with group all scope, no namespace",
				},
			},
		},
		{
			Description:     "Rover Resources <group>--<team>--<name>, and <env> from business context",
			PathParameters:  ":name",
			RequestResource: "", // no additional resource needed
			Templates: map[ClientType]ComparisonTemplates{
				"team": {
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--{{ .B.Team }}--",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Name }}",
					MatchType:         MatchTypePrefix,
				},
				"group": {
					ExpectedTemplate:  "{{ .B.Environment }}--{{ .B.Group }}--",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Name }}",
					MatchType:         MatchTypePrefix,
				},
				"admin": {
					ExpectedTemplate:  "{{ .B.Environment }}--",
					UserInputTemplate: "{{ .B.Environment }}--{{ .P.Name }}",
					MatchType:         MatchTypePrefix,
				},
			},
			TestCases: []TestCase{
				// Team Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "group--team--rover-id",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team--/, group:group, team:team",
					Description:    "Valid token with group and team, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "group--rover-id",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid Token with group and team, namespace mismatch (no team or resourceId)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   "group--team--rover-id",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team--/, group:group, team:team",
					Description:    "Valid token with obfuscated team scope, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "othergroup--rover-id",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group, no team or resourceId)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "group2--rover-id",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (extended group name, no team or resourceId)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "group--teamplayer--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (extended team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "group--te--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (shortened team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "group--foo--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different team)",
				},
				// Group Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "group--foo--resourceId",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace (different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "group2--team--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (extended group name, same team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "gr--team--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (shortened group name, same team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "foo--bar--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (different group, different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "foo--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group, no team or resourceId)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "group2--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (extended group name, no team or resourceId)",
				},
				// Admin Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "group--team--resourceId",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:all"}),
					ResourcePath:   "group--team--resourceId",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin all scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "foo--team--resourceId",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "POST-request with valid token with admin read scope, namespace mismatch (different group)",
				},

				// Global

				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team--/, group:group, team:team",
					Description:    "Valid token with group and team, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--, group:group, team:team",
					Description:    "Valid token with group scope, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--, group:group, team:team",
					Description:    "Valid token with admin scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:all"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test--group--team--/, group:group, team:team",
					Description:    "[POST] Valid token with team all scope, no namespace",
				},
			},
		},
		{
			Description:     "Organization-Server Setup <env>/<group>--<team> and <env>/<group>",
			PathParameters:  ":namespace/:name",
			RequestResource: "", // no additional resource needed
			Templates: map[ClientType]ComparisonTemplates{
				ClientTypeTeam: {
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}--{{ .B.Team }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					MatchType:         MatchTypeEqual,
				},
				ClientTypeGroup: {
					ExpectedTemplate:  "{{ .B.Environment }}/{{ .B.Group }}{{ if contains .P.Name \"--\"}}--{{ lastPart .P.Name \"--\"}}{{ end }}",
					UserInputTemplate: "{{ .P.Namespace }}/{{ .P.Name }}",
					MatchType:         MatchTypeEqual,
				},
				ClientTypeAdmin: {
					ExpectedTemplate:  "{{ .B.Environment }}/",
					UserInputTemplate: "{{ .P.Namespace }}/",
					MatchType:         MatchTypePrefix,
				},
			},
			TestCases: []TestCase{
				// Team Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   env + "/group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with group and team, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "", nil),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token with group but no team, namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "", "", nil),
					ResourcePath:   env + "/group",
					ExpectedStatus: 403,
					ExpectedBody:   "Invalid authorization context",
					Description:    "Invalid token with no group and no team, namespace mismatch (no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--team",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with obfuscated team scope, matching namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--teamplayer",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (extended team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--te",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (shortened team name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:obfuscated"}),
					ResourcePath:   env + "/group--foo",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with obfuscated team scope, mismatching team (different team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   env + "/othergroup--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with team scope, namespace mismatch (different group)",
				},
				// Group Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group--foo",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--foo, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace (different team) and team request",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group, group:group, team:team",
					Description:    "Valid token with group scope, matching namespace (no team) and group request",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/grouper",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, matching namespace (no team) and group request (longer name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/othergroup--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (different group, no team)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/gr--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (shortened group name)",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   env + "/group2--team",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "Valid token with group scope, namespace mismatch (extended group name)",
				},
				// Admin Scope
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:all"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "[POST] Valid token with admin all scope, matching namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   env + "/othergroup",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with admin read scope, but post request",
				},

				// Global

				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "Valid token with group and team, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"group:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group, group:group, team:team",
					Description:    "Valid token with group scope, no namespace",
				},
				{
					Method:         "GET",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"admin:read"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/, group:group, team:team",
					Description:    "Valid token with admin scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:read"}),
					ResourcePath:   "",
					ExpectedStatus: 403,
					ExpectedBody:   "Access to requested resource not allowed",
					Description:    "[POST] Valid token with team scope, no namespace",
				},
				{
					Method:         "POST",
					AccessToken:    mock.NewMockAccessToken("test", "group", "team", []string{"team:all"}),
					ResourcePath:   "",
					ExpectedStatus: 200,
					ExpectedBody:   "prefix:test/group--team/, group:group, team:team",
					Description:    "[POST] Valid token with team all scope, no namespace",
				},
			},
		},
	}

	for i := range testEnvironments {
		t.Run(testEnvironments[i].Description, func(tt *testing.T) {
			app := fiber.New()
			defer func(app *fiber.App) {
				err := app.Shutdown()
				if err != nil {
					tt.Fatal(err)
				}
			}(app)

			app.Use(mock.NewJWTMock())
			app.Use(NewBusinessCtxMiddlewareWithOpts(WithScopePrefix(""), WithDefaultScope("team:read")))
			checkAccess := NewCheckAccessMiddlewareWithOpts(WithTemplates(testEnvironments[i].Templates))

			app.All("/testauth/"+testEnvironments[i].PathParameters, checkAccess, handlerMock)
			app.All("/testauth", checkAccess, handlerMock)

			for _, testCase := range testEnvironments[i].TestCases {
				tt.Run(testCase.Description, func(ttt *testing.T) {
					path := "/testauth"
					if testCase.ResourcePath != "" {
						path += "/" + testCase.ResourcePath + testEnvironments[i].RequestResource
					}
					req := httptest.NewRequest(testCase.Method, path, nil)
					req.Header.Set("Authorization", "Bearer "+testCase.AccessToken)

					res, err := app.Test(req, -1)
					if err != nil {
						ttt.Fatal(err)
					}

					if res.StatusCode != testCase.ExpectedStatus {
						ttt.Fatalf("%s: Expected status %d, got %d", testCase.Description, testCase.ExpectedStatus, res.StatusCode)
					}

					var cmp func(a, b string) bool
					if testCase.ExpectedStatus != 200 {
						cmp = strings.Contains
					} else {
						cmp = strings.EqualFold
					}
					b, _ := io.ReadAll(res.Body)
					if !cmp(string(b), testCase.ExpectedBody) {
						ttt.Fatalf("%s: Expected body '%s', got '%s'", testCase.Description, testCase.ExpectedBody, string(b))
					}
				})
			}
		})
	}

}

func TestCheckAccessOpts(t *testing.T) {
	var TestCases = []TestCase{
		{
			Method:         "GET",
			AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
			ResourcePath:   "group--team",
			ExpectedStatus: 403,
			ExpectedBody:   "Access to requested resource not allowed",
			Description:    "Valid token but namespace does not match expected format",
		},
		{
			Method:         "GET",
			AccessToken:    mock.NewMockAccessToken("test", "group", "team", nil),
			ResourcePath:   "foo--group--team",
			ExpectedStatus: 200,
			ExpectedBody:   "prefix:foo--group--team/, group:group, team:team",
			Description:    "Valid token with custom namespace matching expected format",
		},
	}

	app := fiber.New()

	templates := map[ClientType]ComparisonTemplates{
		ClientTypeTeam: {
			ExpectedTemplate:  "foo--{{ .B.Group }}--{{ .B.Team }}",
			UserInputTemplate: "{{ .P.Custompathparam }}",
			MatchType:         MatchTypePrefix,
		}}

	app.Use(mock.NewJWTMock())
	app.Use(NewBusinessCtxMiddlewareWithOpts(WithScopePrefix(""), WithDefaultScope("team:read")))
	checkAccess := NewCheckAccessMiddlewareWithOpts(
		WithPathParamKey("customPathParam"),
		WithTemplates(templates),
		WithExpectedResourcePathFunc(func(bCtx *BusinessContext, _ map[string]string, template map[ClientType]ComparisonTemplates) (string, error) {
			return "foo--" + bCtx.Group + "--" + bCtx.Team, nil
		}),
	)

	app.All("/testauth/:customPathParam", checkAccess, handlerMock)
	app.All("/testauth", checkAccess, handlerMock)

	for _, testCase := range TestCases {
		t.Run(testCase.Description, func(t *testing.T) {
			path := "/testauth/" + testCase.ResourcePath
			req := httptest.NewRequest(testCase.Method, path, nil)
			req.Header.Set("Authorization", "Bearer "+testCase.AccessToken)

			res, err := app.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}

			if res.StatusCode != testCase.ExpectedStatus {
				t.Fatalf("%s: Expected status %d, got %d", testCase.Description, testCase.ExpectedStatus, res.StatusCode)
			}

			var cmp func(a, b string) bool
			if testCase.ExpectedStatus != 200 {
				cmp = strings.Contains
			} else {
				cmp = strings.EqualFold
			}

			b, _ := io.ReadAll(res.Body)
			if !cmp(string(b), testCase.ExpectedBody) {
				t.Fatalf("%s: Expected body '%s', got '%s'", testCase.Description, testCase.ExpectedBody, string(b))
			}
		})
	}
}

func BenchmarkCheckAccess(b *testing.B) {

	app := fiber.New()

	app.Use(mock.NewJWTMock())
	app.Use(NewBusinessCtxMiddlewareWithOpts(WithScopePrefix(""), WithDefaultScope("team:read")))
	checkAccess := NewCheckAccessMiddlewareWithOpts()

	app.All("/testauth/:namespace/:name", checkAccess, handlerMock)
	app.All("/testauth", checkAccess, handlerMock)

	req := httptest.NewRequest("GET", "/testauth", nil)
	req.Header.Set("Authorization", "Bearer "+mock.NewMockAccessToken("test", "group", "team", nil))

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res, err := app.Test(req)
		if err != nil {
			b.Fatal(err)
		}

		if res.StatusCode != 200 {
			b.Fatalf("Expected status 200, got %d", res.StatusCode)
		}
	}

}
