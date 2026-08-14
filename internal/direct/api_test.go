package direct

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginPersistsTokensWithoutPassword(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user/stateless/login" || request.Method != http.MethodPost {
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
		writeAPIResponse(t, response, map[string]any{
			"accessToken": "access-1", "refreshToken": "refresh-1", "expireTime": "2099-01-01T00:00:00Z",
			"mcpEndpoint": server.URL + "/mcp",
		})
	}))
	defer server.Close()

	filePath := filepath.Join(t.TempDir(), "session.json")
	client := NewAPIClient(NewSessionStore(filePath), server.Client())
	if _, err := client.Login(context.Background(), server.URL, "account-1", "password-1"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsAny(string(data), "password-1", "account-1") {
		t.Fatalf("persisted session leaked login credentials: %s", data)
	}
	session, err := NewSessionStore(filePath).Load()
	if err != nil {
		t.Fatal(err)
	}
	if session.MCPEndpoint != server.URL+"/mcp" {
		t.Fatalf("mcpEndpoint = %q", session.MCPEndpoint)
	}
}

func TestSelectOrganizationRefreshesExpiringSession(t *testing.T) {
	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		switch request.URL.Path {
		case "/user/stateless/refresh":
			writeAPIResponse(t, response, map[string]any{
				"accessToken": "access-2", "refreshToken": "refresh-2", "expireTime": "2099-01-01T00:00:00Z",
			})
		case "/user/info":
			if request.Header.Get("Authorization") != "Bearer access-2" || request.Header.Get("Org-id") != "3" {
				t.Errorf("authenticated headers = %#v", request.Header)
			}
			writeAPIResponse(t, response, map[string]any{"id": 123})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, AccessToken: "expired", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, server.Client())
	scope, err := client.SelectOrganization(context.Background(), "3")
	if err != nil {
		t.Fatalf("SelectOrganization() error = %v", err)
	}
	if scope.UserID != "123" || scope.OrgID != "3" || requestCount != 2 {
		t.Fatalf("scope = %+v, requestCount = %d", scope, requestCount)
	}
	session, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if session.AccessToken != "access-2" || session.RefreshToken != "refresh-2" || session.OrgID != "3" || session.UserID != "123" {
		t.Fatalf("saved session = %+v", session)
	}
}

func TestListOrganizationsUsesIndependentSession(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user/tenant/list" || request.Header.Get("Authorization") != "Bearer access-1" {
			t.Errorf("request = %s %#v", request.URL.Path, request.Header)
		}
		writeAPIResponse(t, response, []map[string]any{{"id": 3, "name": "Moda"}})
	}))
	defer server.Close()
	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, server.Client())
	organizations, err := client.ListOrganizations(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizations() error = %v", err)
	}
	list, ok := organizations.([]any)
	if !ok || len(list) != 1 {
		t.Fatalf("organizations = %#v", organizations)
	}
}

func TestListOrganizationsRefreshesOnceOnBusinessTokenError(t *testing.T) {
	requestCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requestCount++
		switch requestCount {
		case 1:
			json.NewEncoder(response).Encode(map[string]any{"code": 133002, "message": "expired"})
		case 2:
			if request.URL.Path != "/user/stateless/refresh" {
				t.Errorf("refresh path = %s", request.URL.Path)
			}
			writeAPIResponse(t, response, map[string]any{
				"accessToken": "access-2", "refreshToken": "refresh-2", "expireTime": "2099-01-01T00:00:00Z",
			})
		case 3:
			if request.Header.Get("Authorization") != "Bearer access-2" {
				t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
			}
			writeAPIResponse(t, response, []map[string]any{{"id": 3}})
		default:
			t.Errorf("unexpected request %d", requestCount)
		}
	}))
	defer server.Close()
	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, server.Client())
	if _, err := client.ListOrganizations(context.Background()); err != nil {
		t.Fatalf("ListOrganizations() error = %v", err)
	}
	if requestCount != 3 {
		t.Fatalf("requestCount = %d", requestCount)
	}
}

func TestRequestCallsSelectedOrganizationAPI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/store/page" || request.Method != http.MethodPost {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer access-1" || request.Header.Get("Org-id") != "3" {
			t.Errorf("authenticated headers = %#v", request.Header)
		}
		if got := request.URL.Query()["status"]; len(got) != 2 || got[0] != "active" || got[1] != "paused" {
			t.Errorf("status query = %#v", got)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["page"] != float64(2) {
			t.Errorf("request body = %#v", body)
		}
		writeAPIResponse(t, response, map[string]any{"records": []any{}})
	}))
	defer server.Close()

	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour), OrgID: "3", UserID: "123",
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, server.Client())
	result, err := client.Request(context.Background(), APIRequest{
		Method: http.MethodPost,
		Path:   "/store/page",
		Query:  map[string]any{"status": []any{"active", "paused"}},
		Body:   map[string]any{"page": 2},
	})
	if err != nil {
		t.Fatalf("Request() error = %v", err)
	}
	if records, ok := asRecord(result)["records"].([]any); !ok || len(records) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestRequestRejectsMissingScopeAndExternalURL(t *testing.T) {
	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: "https://api.moda.test", AccessToken: "access-1", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, nil)
	if _, err := client.Request(context.Background(), APIRequest{Method: http.MethodGet, Path: "/store/page"}); err == nil || !strings.Contains(err.Error(), "organization") {
		t.Fatalf("missing scope error = %v", err)
	}

	if err := store.Save(Session{
		APIBaseURL: "https://api.moda.test", AccessToken: "access-1", RefreshToken: "refresh-1",
		ExpiresAt: time.Now().Add(time.Hour), OrgID: "3", UserID: "123",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Request(context.Background(), APIRequest{Method: http.MethodGet, Path: "https://attacker.invalid/steal"}); err == nil || !strings.Contains(err.Error(), "relative") {
		t.Fatalf("external URL error = %v", err)
	}
}

func TestConcurrentRequestsShareOneRefresh(t *testing.T) {
	var refreshCount atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/user/stateless/refresh":
			refreshCount.Add(1)
			time.Sleep(50 * time.Millisecond)
			writeAPIResponse(t, response, map[string]any{
				"accessToken": "access-2", "refreshToken": "refresh-2", "expireTime": "2099-01-01T00:00:00Z",
			})
		case "/user/tenant/list":
			if request.Header.Get("Authorization") == "Bearer access-1" {
				json.NewEncoder(response).Encode(map[string]any{"code": 133002, "message": "expired"})
				return
			}
			writeAPIResponse(t, response, []map[string]any{{"id": 3}})
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	store := NewSessionStore(filepath.Join(t.TempDir(), "session.json"))
	if err := store.Save(Session{
		APIBaseURL: server.URL, AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	client := NewAPIClient(store, server.Client())
	var wait sync.WaitGroup
	errorsByCall := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := client.ListOrganizations(context.Background())
			errorsByCall <- err
		}()
	}
	wait.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		if err != nil {
			t.Fatalf("ListOrganizations() error = %v", err)
		}
	}
	if refreshCount.Load() != 1 {
		t.Fatalf("refreshCount = %d, want 1", refreshCount.Load())
	}
}

func writeAPIResponse(t *testing.T, response http.ResponseWriter, data any) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(map[string]any{"code": 200, "data": data}); err != nil {
		t.Fatal(err)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if candidate != "" && strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
