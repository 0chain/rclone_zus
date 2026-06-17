# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build Commands

All builds require `CGO_ENABLED=1` and the `bn256` build tag. CGO is needed because `0chain/gosdk` depends on `herumi/bls-go-binary` (BN254 BLS crypto via C++). There is no pure-Go BN254 BLS library available — switching would require migrating the entire 0chain ecosystem to BLS12-381.

Linux builds use fully static linking (`-extldflags '-static'`), producing one portable binary per architecture.

```bash
make build              # Current platform
make build-linux        # Linux x86_64 (static)
make build-linux-arm64  # Linux ARM64 (static, needs aarch64-linux-gnu-gcc)
make build-mac-arm      # Apple Silicon
make build-mac-amd      # Intel Mac
make build-windows      # Windows cross-compile (needs g++-mingw-w64-x86-64)
```

## Testing

```bash
go test -tags bn256 ./backend/zus/               # Züs backend tests
go test -tags bn256 -run TestFunctionName ./backend/zus/  # Single test
```

Integration tests require remote `automation:` with valid `~/.zcn/` credentials. System tests are in the `system_test` repo under `tests/cli_tests/rclone_zus_tests/`.

## Architecture

`rclone.go` → imports Züs backend → delegates to rclone CLI framework.

`backend/zus/zus.go` implements `fs.Fs`, `fs.Object`, `fs.Purger`, `fs.ListRer`, `fs.Abouter`, `fs.MimeTyper`.

`backend/ram/` is an in-memory reference backend.

## Key Implementation Details

### Wallet mutex for multi-remote support
The gosdk uses global wallet state (`client.wallet`). When multiple remotes are open (cross-allocation transfers), `activateWallet()`/`deactivateWallet()` lock a global mutex and switch the active wallet before each blobber operation. Internal methods (`list`, `newObject`, `update`, `remove`) don't lock — they're called from already-locked contexts.

### Path encoding
Comprehensive encoding flags (EncodeInvalidUtf8, EncodeCtl, EncodeSlash, EncodeDot, EncodeCrLf, etc.) are declared. Encoding is applied at the rclone-SDK boundary: `FromStandardPath()` when sending to SDK, `ToStandardPath()` when returning to rclone.

### Parent directory creation
The SDK does not auto-create parent directories. `ensureParentDirs()` checks and creates them before uploads.

### Zero-length files
The SDK stores 0-byte files as 32-byte MD5 hash content. `readMetaData` detects this and reports size 0. `Open` returns an empty reader for zero-length files.

### Split-key wallets
The wallet splitting is done externally (via Blimp/Vult app which handles zvault registration and key distribution to zauth). rclone_zus only needs the pre-split `wallet.json` (`is_split: true`) and `zauth_server` URL in `config.yaml`. The code detects `IsSplit` and calls `RegisterZauthServer()` which plugs in zauth signing callbacks. All subsequent signing automatically uses the split-key protocol (local partial sign → zauth co-signs with remaining splits).

### Known test limitations (2 of 57 fail)
- `FsIsFile` — NewFs file detection fails for deeply nested paths with encoded special characters
- `ObjectOpenRange` — SDK returns stale content on range read immediately after file update
