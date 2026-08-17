# moda-cli

`moda-cli` is the public Go command-line interface for exposing Moda capabilities to external agents through the Model Context Protocol (MCP).

The project is intentionally independent from the private Moda application repositories. It must not import or copy private Agent Run orchestration, internal Action Catalog implementations, or Electron source code.

## Modes

- `client`: discover a running Moda Electron client and proxy its current authenticated MCP capabilities, including client-side tools such as 3D design. This mode is implemented in this repository and requires an Electron build that publishes the v1 descriptor below.
- `direct`: **not implemented**. Its command names are reserved, but every direct/auth entry point currently fails explicitly without reading credentials, creating a session, or making a network request.

## Client mode

Configure an MCP host to start:

```sh
moda-cli mcp --mode client
```

For one-shot inspection and calls:

```sh
moda-cli tools --mode client
moda-cli call <tool-name> --mode client --input '{}'
```

The command discovers the running desktop client, connects to its loopback HTTPS Streamable HTTP MCP endpoint, and exposes the tools from that endpoint over stdio MCP. Authentication, organization selection, backend access, confirmation UI, and renderer capabilities remain owned by the desktop client. `moda-cli` does not contain a Moda tool catalog and does not receive backend login credentials.

Long-running stdio bridges securely reread the selected descriptor before each request so Electron can rotate its endpoint-scoped token. A refreshed descriptor is accepted only while it still identifies the same Electron process, endpoint, and root CA. If Electron restarts or the connection identity changes, the bridge fails explicitly and must be restarted.

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

The Electron publisher and Go reader share these protocol requirements:

1. Emit `protocolVersion`, `transport`, `expiresAt`, and nested `client.pid/client.startedAt` using the v1 shape above. Transitional top-level `version` and `processId` fields are not accepted.
2. Activate each new endpoint-scoped token before atomically replacing the descriptor. Keep earlier published tokens valid until their own descriptor expiry so another process can observe only an old-valid or new-valid connection pair.
3. Refresh the descriptor before expiry and keep Unix mode `0600` (or a current-user-only Windows ACL). A publication failure must leave the previous descriptor and token usable until their declared expiry.
4. Keep the endpoint at `/agent-runtime/v1/mcp/client`, loopback-only HTTPS, POST-compatible Streamable HTTP, and require `Authorization: Bearer <localAccessToken>`. Descriptor tokens must not authorize any other Agent Runtime path.
5. Continue deriving tools and calls from the existing Electron capability gateway and renderer implementations. Do not add a second 3D implementation or a Go-side business tool catalog.
6. Keep the Electron publisher contract independent from the removed transitional TypeScript `packages/client-cli` and from the future direct-mode architecture.

## Direct mode (not implemented)

Direct mode is not currently available. The CLI reserves `--mode direct` and the `auth` command group, but invoking any of them returns a `not_implemented` error. There is no fallback to client mode.

```sh
moda-cli tools --mode direct
```

The implementation experiments under `internal/direct` are intentionally retained and continue to compile and run their tests, but they are not a supported backend contract. In particular, the prototype `mcpEndpoint` login field is not available from the current Moda backend and must not be treated as deployed functionality.

The intended direct mode is Electron-independent: `moda-cli` will own authentication, token refresh, and organization selection, then call versioned public Open Platform APIs directly. Agent capabilities must come only from explicitly published public operations; the implementation must not copy the private Agent Server catalog, expose arbitrary backend APIs, or include Electron-only tools such as Studio 3D design. This architecture must be agreed with the backend before the entry point is enabled.

## Development

Requires Go 1.24 or newer.

```sh
go test ./...
go vet ./...
go build ./cmd/moda-cli
go run ./cmd/moda-cli --help
```

The public protocol and command surface are being implemented incrementally with tests first.
