package clientconn

import (
	"errors"
	"path/filepath"
	"strings"
)

const FileName = "moda-client-mcp.json"

func ResolveFilePath(explicit string, environment map[string]string, homeDirectory, platform string) (string, error) {
	if override := strings.TrimSpace(explicit); override != "" {
		return filepath.Abs(override)
	}
	if override := strings.TrimSpace(environment["MODA_CLIENT_MCP_CONNECTION_FILE"]); override != "" {
		return filepath.Abs(override)
	}
	if strings.TrimSpace(homeDirectory) == "" {
		return "", errors.New("cannot resolve the Moda client connection descriptor without a home directory")
	}

	switch platform {
	case "darwin":
		return filepath.Join(homeDirectory, "Library", "Application Support", "Moda", FileName), nil
	case "windows":
		baseDirectory := strings.TrimSpace(environment["APPDATA"])
		if baseDirectory == "" {
			baseDirectory = filepath.Join(homeDirectory, "AppData", "Roaming")
		}
		return filepath.Join(baseDirectory, "Moda", FileName), nil
	default:
		baseDirectory := strings.TrimSpace(environment["XDG_CONFIG_HOME"])
		if baseDirectory == "" {
			baseDirectory = filepath.Join(homeDirectory, ".config")
		}
		return filepath.Join(baseDirectory, "Moda", FileName), nil
	}
}
