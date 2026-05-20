# @obedience-corp/tcount

npm distribution wrapper for the `tcount` CLI — a fast, zero-network token counter for LLM workflows.

This package provides the `tcount` command via npm / pnpm / bun.

## Installation

```bash
npm install -g @obedience-corp/tcount
# or
pnpm add -g @obedience-corp/tcount
# or
bun add -g @obedience-corp/tcount
```

After installation, the `tcount` command will be available in your PATH.

## How it works

The package is a lightweight wrapper. On first `postinstall` (or when the binary is missing), it downloads the matching platform-specific release archive from the [tcount GitHub releases](https://github.com/lancekrogers/tcount/releases) and verifies the checksum.

This keeps the npm package small while ensuring you always get the official, signed-off binaries produced by the Go release process.

## Supported Platforms

- macOS (Intel + Apple Silicon)
- Linux (x64 + arm64)

Windows users: use `go install github.com/lancekrogers/tcount/cmd/tcount@latest` or download the `.zip` from the releases page.

## License

MIT
