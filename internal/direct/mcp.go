package direct

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type MCPClient struct {
	session *mcp.ClientSession
}

func ConnectMCP(ctx context.Context, apiClient *APIClient) (*MCPClient, error) {
	session, err := apiClient.ensureFreshSession(ctx)
	if err != nil {
		return nil, err
	}
	if session.OrgID == "" || session.UserID == "" {
		return nil, errors.New("select a Moda organization before using direct mode")
	}
	if session.MCPEndpoint == "" {
		return nil, errors.New("Moda login session does not provide a direct mcpEndpoint")
	}
	baseTransport := apiClient.httpClient.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}
	httpClient := &http.Client{
		Transport: &mcpAuthorizationTransport{apiClient: apiClient, base: baseTransport},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Moda direct MCP endpoint must not redirect")
		},
	}
	transport := &mcp.StreamableClientTransport{
		Endpoint: session.MCPEndpoint, HTTPClient: httpClient, MaxRetries: -1, DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "moda-cli-direct", Version: "0.1.0"}, nil)
	mcpSession, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect to Moda direct MCP endpoint: %w", err)
	}
	return &MCPClient{session: mcpSession}, nil
}

func (client *MCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	var tools []*mcp.Tool
	seen := make(map[string]struct{})
	cursor := ""
	for {
		result, err := client.session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list direct Moda tools: %w", err)
		}
		tools = append(tools, result.Tools...)
		if result.NextCursor == "" {
			return tools, nil
		}
		if _, exists := seen[result.NextCursor]; exists {
			return nil, fmt.Errorf("Moda direct MCP repeated tools/list cursor %q", result.NextCursor)
		}
		seen[result.NextCursor] = struct{}{}
		cursor = result.NextCursor
	}
}

func (client *MCPClient) CallTool(ctx context.Context, name string, arguments any) (*mcp.CallToolResult, error) {
	result, err := client.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return nil, fmt.Errorf("call direct Moda tool %q: %w", name, err)
	}
	return result, nil
}

func (client *MCPClient) RunBridge(ctx context.Context, agentTransport mcp.Transport) error {
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	bridge := mcp.NewServer(&mcp.Implementation{Name: "moda-direct-bridge", Version: "0.1.0"}, nil)
	registered := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool == nil || tool.Name == "" || tool.InputSchema == nil {
			return errors.New("Moda direct MCP returned a tool without a name or input schema")
		}
		if _, exists := registered[tool.Name]; exists {
			return fmt.Errorf("Moda direct MCP returned duplicate tool %q", tool.Name)
		}
		registered[tool.Name] = struct{}{}
		remoteTool := tool
		bridge.AddTool(remoteTool, func(callContext context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return client.CallTool(callContext, remoteTool.Name, request.Params.Arguments)
		})
	}
	if err := bridge.Run(ctx, agentTransport); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run direct MCP bridge: %w", err)
	}
	return nil
}

func (client *MCPClient) Close() error {
	return client.session.Close()
}

type mcpAuthorizationTransport struct {
	apiClient *APIClient
	base      http.RoundTripper
}

func (transport *mcpAuthorizationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	session, err := transport.apiClient.ensureFreshSession(request.Context())
	if err != nil {
		return nil, err
	}
	response, err := transport.base.RoundTrip(authenticatedMCPRequest(request, session))
	if err != nil || (response.StatusCode != http.StatusUnauthorized && response.StatusCode != http.StatusForbidden) {
		return response, err
	}
	response.Body.Close()
	refreshed, err := transport.apiClient.refresh(request.Context(), session)
	if err != nil {
		return nil, err
	}
	retry := authenticatedMCPRequest(request, refreshed)
	if request.GetBody != nil {
		retry.Body, err = request.GetBody()
		if err != nil {
			return nil, err
		}
	}
	return transport.base.RoundTrip(retry)
}

func authenticatedMCPRequest(request *http.Request, session Session) *http.Request {
	requestCopy := request.Clone(request.Context())
	requestCopy.Header = request.Header.Clone()
	requestCopy.Header.Set("Authorization", "Bearer "+session.AccessToken)
	requestCopy.Header.Set("Org-id", session.OrgID)
	return requestCopy
}
