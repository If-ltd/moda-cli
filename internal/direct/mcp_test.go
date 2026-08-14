package direct

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConnectMCPUsesSessionTokenAndOrganization(t *testing.T) {
	remote := mcp.NewServer(&mcp.Implementation{Name: "direct-test", Version: "1.0.0"}, nil)
	remote.AddTool(&mcp.Tool{
		Name: "moda_store_list", InputSchema: map[string]any{"type": "object"},
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var arguments map[string]any
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{StructuredContent: map[string]any{"page": arguments["page"]}}, nil
	})
	handler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return remote },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access-1" || request.Header.Get("Org-id") != "3" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(response, request)
	}))
	defer server.Close()

	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, MCPEndpoint: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour), OrgID: "3", UserID: "123",
	}); err != nil {
		t.Fatal(err)
	}
	apiClient := NewAPIClient(store, server.Client())
	client, err := ConnectMCP(context.Background(), apiClient)
	if err != nil {
		t.Fatalf("ConnectMCP() error = %v", err)
	}
	defer client.Close()
	tools, err := client.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "moda_store_list" {
		t.Fatalf("tools = %+v, error = %v", tools, err)
	}
	result, err := client.CallTool(context.Background(), "moda_store_list", map[string]any{"page": 2})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["page"] != float64(2) {
		t.Fatalf("result = %#v", result.StructuredContent)
	}
}

func TestConnectMCPRefreshesOnceAfterUnauthorized(t *testing.T) {
	remote := mcp.NewServer(&mcp.Implementation{Name: "direct-refresh-test", Version: "1.0.0"}, nil)
	remote.AddTool(&mcp.Tool{Name: "moda_store_list", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return remote },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
	refreshCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/user/stateless/refresh" {
			refreshCount++
			writeAPIResponse(t, response, map[string]any{
				"accessToken": "access-2", "refreshToken": "refresh-2", "expireTime": "2099-01-01T00:00:00Z",
			})
			return
		}
		if request.Header.Get("Authorization") != "Bearer access-2" {
			http.Error(response, "expired", http.StatusUnauthorized)
			return
		}
		streamable.ServeHTTP(response, request)
	}))
	defer server.Close()
	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, MCPEndpoint: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour), OrgID: "3", UserID: "123",
	}); err != nil {
		t.Fatal(err)
	}
	client, err := ConnectMCP(context.Background(), NewAPIClient(store, server.Client()))
	if err != nil {
		t.Fatalf("ConnectMCP() error = %v", err)
	}
	defer client.Close()
	if _, err := client.ListTools(context.Background()); err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if refreshCount != 1 {
		t.Fatalf("refreshCount = %d, want 1", refreshCount)
	}
}
