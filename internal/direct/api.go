package direct

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Scope struct {
	UserID string `json:"userId"`
	OrgID  string `json:"orgId"`
}

type APIClient struct {
	store      *SessionStore
	httpClient *http.Client
	now        func() time.Time
	refreshMu  sync.Mutex
}

func NewAPIClient(store *SessionStore, httpClient *http.Client) *APIClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &APIClient{store: store, httpClient: httpClient, now: time.Now}
}

func (client *APIClient) Login(ctx context.Context, apiBaseURL, account, password string) (Session, error) {
	baseURL, err := normalizeAPIBaseURL(apiBaseURL)
	if err != nil {
		return Session{}, err
	}
	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	if account == "" || password == "" {
		return Session{}, errors.New("account and password are required")
	}
	responseData, err := client.request(ctx, baseURL+"/user/stateless/login", http.MethodPost, map[string]any{
		"account": account, "password": password,
	}, "", "")
	if err != nil {
		return Session{}, fmt.Errorf("login to Moda: %w", err)
	}
	data := asRecord(responseData)
	session := Session{
		APIBaseURL:   baseURL,
		MCPEndpoint:  requiredText(data["mcpEndpoint"]),
		AccessToken:  requiredText(data["accessToken"]),
		RefreshToken: requiredText(data["refreshToken"]),
		ExpiresAt:    parseExpiration(data["expireTime"]),
	}
	if err := client.store.Save(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (client *APIClient) SelectOrganization(ctx context.Context, orgID string) (Scope, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return Scope{}, errors.New("orgId is required")
	}
	session, err := client.ensureFreshSession(ctx)
	if err != nil {
		return Scope{}, err
	}
	responseData, err := client.authenticatedRequest(ctx, session, "/user/info", http.MethodGet, nil, orgID, true)
	if err != nil {
		return Scope{}, err
	}
	data := asRecord(responseData)
	userID := requiredText(data["id"])
	if userID == "" {
		userID = requiredText(data["userId"])
	}
	if userID == "" {
		return Scope{}, errors.New("user.id is required")
	}
	session.OrgID = orgID
	session.UserID = userID
	if err := client.store.Save(session); err != nil {
		return Scope{}, err
	}
	return Scope{UserID: userID, OrgID: orgID}, nil
}

func (client *APIClient) ListOrganizations(ctx context.Context) (any, error) {
	session, err := client.ensureFreshSession(ctx)
	if err != nil {
		return nil, err
	}
	return client.authenticatedRequest(ctx, session, "/user/tenant/list", http.MethodGet, nil, "", true)
}

func (client *APIClient) authenticatedRequest(
	ctx context.Context,
	session Session,
	pathname, method string,
	body any,
	orgID string,
	allowRefresh bool,
) (any, error) {
	result, err := client.request(ctx, session.APIBaseURL+pathname, method, body, session.AccessToken, orgID)
	if err == nil || !allowRefresh || !shouldRefresh(err) {
		return result, err
	}
	refreshed, refreshErr := client.refresh(ctx, session)
	if refreshErr != nil {
		return nil, refreshErr
	}
	return client.authenticatedRequest(ctx, refreshed, pathname, method, body, orgID, false)
}

func (client *APIClient) ensureFreshSession(ctx context.Context) (Session, error) {
	session, err := client.store.Load()
	if err != nil {
		return Session{}, err
	}
	if !session.ExpiresAt.IsZero() && !session.ExpiresAt.After(client.now().Add(time.Minute)) {
		return client.refresh(ctx, session)
	}
	return session, nil
}

func (client *APIClient) refresh(ctx context.Context, session Session) (Session, error) {
	client.refreshMu.Lock()
	defer client.refreshMu.Unlock()
	current, err := client.store.Load()
	if err == nil && current.AccessToken != session.AccessToken {
		return current, nil
	}
	responseData, err := client.request(ctx, session.APIBaseURL+"/user/stateless/refresh", http.MethodPost, map[string]any{
		"refreshToken": session.RefreshToken,
	}, "", "")
	if err != nil {
		return Session{}, fmt.Errorf("refresh Moda session: %w", err)
	}
	data := asRecord(responseData)
	session.AccessToken = requiredText(data["accessToken"])
	if replacement := requiredText(data["refreshToken"]); replacement != "" {
		session.RefreshToken = replacement
	}
	if expiresAt := parseExpiration(data["expireTime"]); !expiresAt.IsZero() {
		session.ExpiresAt = expiresAt
	}
	if err := client.store.Save(session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (client *APIClient) request(ctx context.Context, endpoint, method string, body any, accessToken, orgID string) (any, error) {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		requestBody = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if orgID != "" {
		request.Header.Set("Org-id", orgID)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode Moda API response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || (envelope.Code != 0 && envelope.Code != 200) {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = fmt.Sprintf("Moda API request failed with HTTP %d", response.StatusCode)
		}
		return nil, &apiError{message: message, code: envelope.Code, statusCode: response.StatusCode}
	}
	var result any
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		result = map[string]any{}
	} else if err := json.Unmarshal(envelope.Data, &result); err != nil {
		return nil, fmt.Errorf("decode Moda API data: %w", err)
	}
	return result, nil
}

type apiError struct {
	message    string
	code       int
	statusCode int
}

func (err *apiError) Error() string { return err.message }

func shouldRefresh(err error) bool {
	var candidate *apiError
	return errors.As(err, &candidate) &&
		(candidate.code == 133002 || candidate.statusCode == http.StatusUnauthorized || candidate.statusCode == http.StatusForbidden)
}

func asRecord(value any) map[string]any {
	if record, ok := value.(map[string]any); ok {
		return record
	}
	return map[string]any{}
}

func requiredText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func parseExpiration(value any) time.Time {
	text := requiredText(value)
	if text == "" {
		return time.Time{}
	}
	if number, err := strconv.ParseInt(text, 10, 64); err == nil {
		if number < 1_000_000_000_000 {
			number *= 1000
		}
		return time.UnixMilli(number).UTC()
	}
	parsed, _ := time.Parse(time.RFC3339Nano, text)
	return parsed
}
