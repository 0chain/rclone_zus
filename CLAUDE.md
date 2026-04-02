# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

rclone_zus is a custom out-of-tree backend for [rclone](https://rclone.org/) that integrates with **Züs**, a blockchain-based decentralized cloud storage platform. It implements rclone's filesystem interface to enable standard rclone commands (copy, sync, move, ls, etc.) against Züs allocations.

**Module:** `github.com/0chain/rclone_zus`

## Build Commands

All builds require `CGO_ENABLED=1` and the `bn256` build tag. CGO is needed because `0chain/gosdk` depends on `herumi/bls-go-binary` (native C/C++ BLS crypto).

**Linux builds use fully static linking** (`-extldflags '-static'`), producing a single portable binary per architecture that works on any Linux distro (no glibc version dependency). This mirrors rclone's own approach of shipping one Linux binary. macOS and Windows use system libc (stable ABI).

```bash
# Default build (current platform)
make build

# Linux (fully static, portable)
make build-linux          # x86_64
make build-linux-arm64    # ARM64 (requires aarch64-linux-gnu-gcc)

# macOS (CGO with system libc)
make build-mac-arm        # Apple Silicon
make build-mac-amd        # Intel Mac

# Windows
make build-windows        # Cross-compile with MinGW (requires g++-mingw-w64-x86-64)
make build-windows-native # Native on Windows
```

## Testing

```bash
# Run all tests
go test -tags bn256 ./...

# Run Züs backend tests
go test -tags bn256 ./backend/zus/

# Run RAM backend tests
go test -tags bn256 ./backend/ram/

# Run a single test
go test -tags bn256 -run TestFunctionName ./backend/zus/
```

Integration tests require a configured remote named `automation:` with valid Züs wallet/allocation credentials in `~/.zcn/`.

## Architecture

### Entry Point

`rclone.go` — minimal main that imports the Züs backend and delegates to rclone's CLI framework.

### Backends (`backend/`)

- **`zus/`** — Primary backend. `zus.go` implements rclone's `fs.Fs` and `fs.Object` interfaces against the Züs network via `0chain/gosdk`. Supports server-side copy/move (blobber-native), batched operations, optional encryption, range downloads, and custom metadata for rclone modification times.
- **`ram/`** — In-memory reference backend for testing/development.

### Key Dependencies

- `github.com/0chain/gosdk` — Züs SDK for wallet init, allocation management, file operations
- `github.com/rclone/rclone` — Core rclone framework (filesystem interfaces, CLI, config)

### Züs Configuration

Requires files in `~/.zcn/`:
- `wallet.json` — Züs wallet credentials
- `config.yaml` — Züs network configuration
- `allocation.txt` — Allocation ID

### Batching

The Züs backend supports batched file operations via `batcher.go` to reduce network round-trips. Configurable modes: `sync` (default), `async`, `off`. Default batch size: 50, timeout: 500ms. Batch operations execute via `alloc.DoMultiOperation()`.

### Cross-wallet/allocation transfers

rclone natively supports copying between different Züs wallets/allocations via two-step staging (download from source, upload to destination). Direct cross-wallet transfers in a single process are not supported because the gosdk uses global wallet state. Configure separate remotes for each wallet/allocation.

### Split-key wallet support

Split-key wallets are supported. Set `"is_split": true` in `wallet.json` and add `zauth_server: <url>` to `config.yaml`. The backend automatically registers the zauth server for signing operations.

### Path encoding

The backend declares `EncodeInvalidUtf8` encoding to handle invalid UTF-8 characters in filenames. Paths are encoded/decoded at the rclone-SDK boundary.

### Known SDK limitations

- The SDK does not auto-create parent directories; the backend calls `ensureParentDirs()` before uploads
- Server-side copy does not support overwriting existing files
- The SDK uses global wallet state, preventing simultaneous multi-wallet operations in one process
