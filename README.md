# moda-cli

`moda-cli` is the public Go command-line interface for exposing Moda capabilities to external agents through the Model Context Protocol (MCP).

The project is intentionally independent from the private Moda application repositories. It must not import or copy private Agent Run orchestration, internal Action Catalog implementations, or Electron source code.

## Modes

- `client`: discover a running Moda Electron client and proxy its current authenticated MCP capabilities, including client-side tools such as 3D design. This mode is implemented in this repository and requires an Electron build that publishes the v1 descriptor below.
- `direct`: maintain an independent Moda login session and connect to a remote Moda MCP endpoint. Direct mode has no Electron-only capabilities and is not implemented yet.

## Client mode

Configure an MCP host to start:

```sh
moda-cli mcp --mode client
```

The command discovers the running desktop client, connects to its loopback HTTPS Streamable HTTP MCP endpoint, and exposes the tools from that endpoint over stdio MCP. Authentication, organization selection, backend access, confirmation UI, and renderer capabilities remain owned by the desktop client. `moda-cli` does not contain a Moda tool catalog and does not receive backend login credentials.

Use `--connection-file FILE` for development instances or another non-default descriptor. The flag takes precedence over `MODA_CLIENT_MCP_CONNECTION_FILE`; neither source silently falls back when the selected file is invalid.

Default descriptor paths are:

- macOS: `~/Library/Application Support/Moda/moda-client-mcp.json`
- Windows: `%APPDATA%\Moda\moda-client-mcp.json`
- Linux: `${XDG_CONFIG_HOME:-~/.config}/Moda/moda-client-mcp.json`

### Connection descriptor v1

Electron publishes this process-local descriptor atomically while its MCP endpoint is available:

```json
{
  "protocol": "moda.client-mcp-connection",
  "protocolVersion": 1,
  "transport": "streamable-http",
  "endpoint": "https://127.0.0.1:61002/agent-runtime/v1/mcp/client",
  "localAccessToken": "process-local-secret",
  "rootCaPath": "/absolute/path/to/local-root-ca.pem",
  "expiresAt": "2026-08-14T12:05:00Z",
  "client": {
    "pid": 1234,
    "startedAt": "2026-08-14T12:00:00Z"
  }
}
```

`protocolVersion` versions the Moda connection descriptor, not the MCP wire protocol. The standard MCP initialize handshake negotiates the wire protocol independently.

On Unix, the descriptor must be a regular, non-symlink file owned by the current user with exact `0600` permissions. On Windows, Electron must apply an equivalent current-user-only ACL because POSIX mode bits are not available. The reader rejects unknown fields, unsupported protocol or transport values, non-loopback/non-HTTPS endpoints, the wrong endpoint path, missing local credentials or CA files, expired descriptors, invalid timestamps, and processes that are no longer running.

The local access token is only for the Electron loopback endpoint. The descriptor must never contain a Moda backend access token, refresh token, organization credential, or private server configuration.

### Electron publisher contract

The current TypeScript publisher is transitional and must be updated in `moda-studio` before this Go client can connect to a released desktop build. The required private-repository changes are deliberately limited to the protocol boundary:

1. Change the descriptor writer to emit `protocolVersion`, `transport`, `expiresAt`, and nested `client.pid/client.startedAt` using the v1 shape above; remove the transitional top-level `version` and `processId` fields.
2. Refresh the descriptor and process-local token before expiry, continue using atomic replacement, and keep Unix mode `0600` (or a current-user-only Windows ACL).
3. Keep the endpoint at `/agent-runtime/v1/mcp/client`, loopback-only HTTPS, POST-compatible Streamable HTTP, and require `Authorization: Bearer <localAccessToken>`.
4. Continue deriving tools and calls from the existing Electron capability gateway and renderer implementations. Do not add a second 3D implementation or a Go-side business tool catalog.
5. Remove the transitional TypeScript `packages/client-cli` only after released Electron builds and this Go CLI have reached capability parity.

## Development

Requires Go 1.24 or newer.

```sh
go test ./...
go vet ./...
go build ./cmd/moda-cli
go run ./cmd/moda-cli --help
```

The public protocol and command surface are being implemented incrementally with tests first.
