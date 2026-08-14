package modacli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/if-ltd/moda-cli/internal/direct"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDirectAuthenticationLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user/stateless/login" {
			http.NotFound(response, request)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["account"] != "account-1" || body["password"] != "password-1" {
			t.Errorf("login body = %#v", body)
		}
		if err := json.NewEncoder(response).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"accessToken": "access-1", "refreshToken": "refresh-1", "expireTime": "2099-01-01T00:00:00Z",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	sessionFile := filepath.Join(t.TempDir(), "session.json")
	login := exec.Command(
		"go", "run", "./cmd/moda-cli", "auth", "login",
		"--api-base-url", server.URL, "--account", "account-1", "--password-stdin", "--session-file", sessionFile,
	)
	login.Stdin = bytes.NewBufferString("password-1\n")
	loginOutput, err := login.CombinedOutput()
	if err != nil {
		t.Fatalf("auth login failed: %v\n%s", err, loginOutput)
	}
	assertJSONField(t, loginOutput, "loggedIn", true)

	status := exec.Command("go", "run", "./cmd/moda-cli", "auth", "status", "--session-file", sessionFile)
	statusOutput, err := status.CombinedOutput()
	if err != nil {
		t.Fatalf("auth status failed: %v\n%s", err, statusOutput)
	}
	assertJSONField(t, statusOutput, "apiBaseUrl", server.URL)

	logout := exec.Command("go", "run", "./cmd/moda-cli", "auth", "logout", "--session-file", sessionFile)
	logoutOutput, err := logout.CombinedOutput()
	if err != nil {
		t.Fatalf("auth logout failed: %v\n%s", err, logoutOutput)
	}
	assertJSONField(t, logoutOutput, "loggedIn", false)
	if _, err := os.Stat(sessionFile); !os.IsNotExist(err) {
		t.Fatalf("session still exists after logout: %v", err)
	}
}

func TestDirectOrganizationCommands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/user/tenant/list":
			json.NewEncoder(response).Encode(map[string]any{"code": 200, "data": []map[string]any{{"id": 3}}})
		case "/user/info":
			if request.Header.Get("Org-id") != "3" {
				t.Errorf("Org-id = %q", request.Header.Get("Org-id"))
			}
			json.NewEncoder(response).Encode(map[string]any{"code": 200, "data": map[string]any{"id": 123}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	sessionFile := filepath.Join(t.TempDir(), "session.json")
	store := direct.NewSessionStore(sessionFile)
	if err := store.Save(direct.Session{
		APIBaseURL: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	orgs := exec.Command("go", "run", "./cmd/moda-cli", "auth", "orgs", "--session-file", sessionFile)
	orgsOutput, err := orgs.CombinedOutput()
	if err != nil {
		t.Fatalf("auth orgs failed: %v\n%s", err, orgsOutput)
	}
	var organizationList []any
	if err := json.Unmarshal(orgsOutput, &organizationList); err != nil || len(organizationList) != 1 {
		t.Fatalf("organizations = %s, error = %v", orgsOutput, err)
	}

	useOrg := exec.Command("go", "run", "./cmd/moda-cli", "auth", "use-org", "3", "--session-file", sessionFile)
	useOrgOutput, err := useOrg.CombinedOutput()
	if err != nil {
		t.Fatalf("auth use-org failed: %v\n%s", err, useOrgOutput)
	}
	assertJSONField(t, useOrgOutput, "orgId", "3")
}

func TestDirectToolsListsRemoteMCPTools(t *testing.T) {
	fixture := newDirectMCPFixture(t)
	command := exec.Command("go", "run", "./cmd/moda-cli", "tools", "--mode", "direct", "--session-file", fixture.sessionFile)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("direct tools failed: %v\n%s", err, output)
	}
	var tools []*mcp.Tool
	if err := json.Unmarshal(output, &tools); err != nil || len(tools) != 1 || tools[0].Name != "moda_store_list" {
		t.Fatalf("tools = %s, error = %v", output, err)
	}
}

func TestDirectCallInvokesRemoteMCPTool(t *testing.T) {
	fixture := newDirectMCPFixture(t)
	command := exec.Command(
		"go", "run", "./cmd/moda-cli", "call", "moda_store_list", "--mode", "direct",
		"--input", `{"page":2}`, "--session-file", fixture.sessionFile,
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("direct call failed: %v\n%s", err, output)
	}
	var result mcp.CallToolResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode direct call: %v\n%s", err, output)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["page"] != float64(2) {
		t.Fatalf("result = %#v", result.StructuredContent)
	}
}

func TestDirectModeRunsStdioBridgeToRemoteMCP(t *testing.T) {
	fixture := newDirectMCPFixture(t)
	command := exec.Command("go", "run", "./cmd/moda-cli", "mcp", "--mode", "direct", "--session-file", fixture.sessionFile)
	agent := mcp.NewClient(&mcp.Implementation{Name: "direct-stdio-test", Version: "1.0.0"}, nil)
	session, err := agent.Connect(context.Background(), &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect to direct stdio bridge: %v", err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 1 || tools.Tools[0].Name != "moda_store_list" {
		t.Fatalf("tools = %+v, error = %v", tools, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "moda_store_list", Arguments: map[string]any{"page": 4},
	})
	if err != nil {
		t.Fatalf("call direct stdio tool: %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["page"] != float64(4) {
		t.Fatalf("result = %#v", result.StructuredContent)
	}
}

type directMCPFixture struct {
	sessionFile string
}

func newDirectMCPFixture(t *testing.T) directMCPFixture {
	t.Helper()
	remote := mcp.NewServer(&mcp.Implementation{Name: "direct-cli-test", Version: "1.0.0"}, nil)
	remote.AddTool(&mcp.Tool{
		Name: "moda_store_list", InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{
			"page": arguments["page"],
		}}, nil
	})
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return remote },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer direct-access" || request.Header.Get("Org-id") != "3" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(response, request)
	}))
	t.Cleanup(server.Close)
	sessionFile := filepath.Join(t.TempDir(), "session.json")
	if err := direct.NewSessionStore(sessionFile).Save(direct.Session{
		APIBaseURL: server.URL, MCPEndpoint: server.URL, AccessToken: "direct-access", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour), OrgID: "3", UserID: "123",
	}); err != nil {
		t.Fatal(err)
	}
	return directMCPFixture{sessionFile: sessionFile}
}

func assertJSONField(t *testing.T, data []byte, field string, want any) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, data)
	}
	if value[field] != want {
		t.Fatalf("%s = %#v, want %#v in %s", field, value[field], want, data)
	}
}
