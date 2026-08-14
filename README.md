# moda-cli

`moda-cli` is the public Go command-line interface for exposing Moda capabilities to external agents through the Model Context Protocol (MCP).

The project is intentionally independent from the private Moda application repositories. It must not import or copy private Agent Run orchestration, internal Action Catalog implementations, or Electron source code.

## Planned modes

- `client`: discover a running Moda Electron client and proxy its current authenticated MCP capabilities, including client-side tools such as 3D design.
- `direct`: maintain an independent Moda login session and connect to a remote Moda MCP endpoint. Direct mode has no Electron-only capabilities.

## Development

Requires Go 1.24 or newer.

```sh
go test ./...
go run ./cmd/moda-cli --help
```

The public protocol and command surface are being implemented incrementally with tests first.
