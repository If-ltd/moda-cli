package direct

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Session struct {
	APIBaseURL   string
	MCPEndpoint  string
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	OrgID        string
	UserID       string
}

type sessionFile struct {
	APIBaseURL   string `json:"apiBaseUrl"`
	MCPEndpoint  string `json:"mcpEndpoint,omitempty"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAtMS  *int64 `json:"expiresAtMs,omitempty"`
	OrgID        string `json:"orgId,omitempty"`
	UserID       string `json:"userId,omitempty"`
}

type SessionStore struct {
	filePath string
}

func NewSessionStore(filePath string) *SessionStore {
	return &SessionStore{filePath: filePath}
}

func (store *SessionStore) Load() (Session, error) {
	info, err := os.Lstat(store.filePath)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, errors.New("Moda CLI direct mode is not logged in")
	}
	if err != nil {
		return Session{}, fmt.Errorf("inspect direct session %q: %w", store.filePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Session{}, errors.New("direct session must be a regular non-symlink file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		return Session{}, errors.New("direct session permissions must be 0600")
	}
	file, err := os.Open(store.filePath)
	if err != nil {
		return Session{}, fmt.Errorf("open direct session %q: %w", store.filePath, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	var wire sessionFile
	if err := decoder.Decode(&wire); err != nil {
		return Session{}, fmt.Errorf("decode direct session %q: %w", store.filePath, err)
	}
	return sessionFromFile(wire)
}

func (store *SessionStore) Save(session Session) error {
	normalized, err := normalizeSession(session)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(store.filePath), 0o700); err != nil {
		return fmt.Errorf("create direct session directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.filePath), ".moda-session-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary direct session: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure temporary direct session: %w", err)
	}
	wire := sessionToFile(normalized)
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(wire); err != nil {
		temporary.Close()
		return fmt.Errorf("encode direct session: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync direct session: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close direct session: %w", err)
	}
	if err := os.Rename(temporaryPath, store.filePath); err != nil {
		return fmt.Errorf("replace direct session: %w", err)
	}
	removeTemporary = false
	return os.Chmod(store.filePath, 0o600)
}

func (store *SessionStore) Clear() error {
	if err := os.Remove(store.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove direct session: %w", err)
	}
	return nil
}

func ResolveSessionFilePath(explicit string, environment map[string]string, homeDirectory, platform string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		return filepath.Abs(value)
	}
	if value := strings.TrimSpace(environment["MODA_CLIENT_CLI_SESSION_FILE"]); value != "" {
		return filepath.Abs(value)
	}
	if strings.TrimSpace(homeDirectory) == "" {
		return "", errors.New("cannot resolve direct session without a home directory")
	}
	base := ""
	switch platform {
	case "darwin":
		base = filepath.Join(homeDirectory, "Library", "Application Support")
	case "windows":
		base = strings.TrimSpace(environment["APPDATA"])
		if base == "" {
			base = filepath.Join(homeDirectory, "AppData", "Roaming")
		}
	default:
		base = strings.TrimSpace(environment["XDG_CONFIG_HOME"])
		if base == "" {
			base = filepath.Join(homeDirectory, ".config")
		}
	}
	return filepath.Join(base, "Moda", "client-cli", "session.json"), nil
}

func normalizeSession(session Session) (Session, error) {
	baseURL, err := normalizeAPIBaseURL(session.APIBaseURL)
	if err != nil {
		return Session{}, err
	}
	session.APIBaseURL = baseURL
	if strings.TrimSpace(session.MCPEndpoint) != "" {
		endpoint, err := normalizeRemoteMCPEndpoint(session.MCPEndpoint)
		if err != nil {
			return Session{}, err
		}
		session.MCPEndpoint = endpoint
	}
	session.AccessToken = strings.TrimSpace(session.AccessToken)
	session.RefreshToken = strings.TrimSpace(session.RefreshToken)
	session.OrgID = strings.TrimSpace(session.OrgID)
	session.UserID = strings.TrimSpace(session.UserID)
	if session.AccessToken == "" || session.RefreshToken == "" {
		return Session{}, errors.New("direct session accessToken and refreshToken are required")
	}
	return session, nil
}

func normalizeAPIBaseURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return "", errors.New("apiBaseUrl is invalid")
	}
	isLocalHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")
	if parsed.Scheme != "https" && !isLocalHTTP {
		return "", errors.New("Moda direct API requires HTTPS except for a local development server")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func sessionToFile(session Session) sessionFile {
	wire := sessionFile{
		APIBaseURL: session.APIBaseURL, MCPEndpoint: session.MCPEndpoint, AccessToken: session.AccessToken, RefreshToken: session.RefreshToken,
		OrgID: session.OrgID, UserID: session.UserID,
	}
	if !session.ExpiresAt.IsZero() {
		value := session.ExpiresAt.UnixMilli()
		wire.ExpiresAtMS = &value
	}
	return wire
}

func sessionFromFile(wire sessionFile) (Session, error) {
	session := Session{
		APIBaseURL: wire.APIBaseURL, MCPEndpoint: wire.MCPEndpoint, AccessToken: wire.AccessToken, RefreshToken: wire.RefreshToken,
		OrgID: wire.OrgID, UserID: wire.UserID,
	}
	if wire.ExpiresAtMS != nil {
		session.ExpiresAt = time.UnixMilli(*wire.ExpiresAtMS).UTC()
	}
	return normalizeSession(session)
}

func normalizeRemoteMCPEndpoint(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return "", errors.New("mcpEndpoint is invalid")
	}
	isLocalHTTP := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")
	if parsed.Scheme != "https" && !isLocalHTTP {
		return "", errors.New("Moda direct MCP requires HTTPS except for a local development server")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("mcpEndpoint must not contain credentials or a fragment")
	}
	return parsed.String(), nil
}
