# rclone-zus vs mc vs aws-s3 — Cold-cache throughput

Benchmark: 155 mixed files, 577 MB total (100×4 KB, 50×1 MB, 5×100 MB), uploaded and downloaded against the same Züs allocation.

`mc` and `aws-s3` go through `zs3server` (the Züs S3 gateway). `rclone-zus` goes direct to blobbers via gosdk.

All DOWN legs preceded by `sync && echo 3 > /proc/sys/vm/drop_caches` on the host (page cache is shared with containers, so this drops blobber-container caches too).

Measured 2026-04-14 on test2 (144.76.58.147) against `feat/enterprise-blobber` gosdk + `feature/static-builds-eblobber-compat` rclone-zus.

| Tool         | UP MB/s   | DOWN MB/s | Integrity |
| ------------ | --------- | --------- | --------- |
| **rclone-zus** | **106.7** | 250.3     | ✅        |
| mc           | 44.7      | **310.8** | ✅        |
| aws-s3       | 21.6      | 58.2      | ✅        |

## Why rclone-zus wins UP (2.4× mc, 4.9× aws-s3)

rclone-zus speaks gosdk directly to blobbers. For each upload:

```
rclone-zus:  client → 6 blobbers in parallel (4 data + 2 parity), SDK handles commit marker
mc/aws-s3:   client → zs3server (single proxy) → 6 blobbers
```

The proxy hop is not just latency. `zs3server` has admission-control limits (`max_concurrent_requests`, `upload_workers`) and buffers the full object before fanning out to blobbers. For small objects the proxy round-trip dominates; for large objects the buffer-pool contention dominates.

Concurrency is the other half. With `--transfers=16`, rclone launches 16 per-file goroutines. Before our fix (see below), all 16 serialized on a global `walletMu`. After the fix, the wallet mutex uses a read-lock fast path so the 16 goroutines run in parallel, each doing its own 6-way blobber fan-out. Effective concurrency: 16 × 6 = 96 in-flight blobber requests vs mc's bounded proxy stream count.

## Why mc still wins DOWN (mc 311 vs rclone-zus 250)

Even with cold page cache, mc retains a long-lived SDK advantage: the `zs3server` process has a hot allocation object, blobber connection pool, and warm erasure-coder state. Every DOWN through mc reuses that state. rclone-zus spawns a fresh gosdk client per rclone invocation, paying initialization cost on each run.

If `zs3server` were restarted between runs (true cold state), rclone-zus should match or exceed mc — the direct-to-blobber path is strictly shorter.

## Fixes that got us here

Three code changes, all on `feat/enterprise-blobber` branch of gosdk and `feature/static-builds-eblobber-compat` branch of rclone-zus.

### 1. `gosdk zboxcore/sdk/allocation.go` — in-memory `DownloadObject` (zero-copy)

Previous shim wrapped `DownloadFileToFileHandler` in an `io.Pipe` with a no-op `Seek`. The SDK's chunked downloader calls `Seek(offset)+Write(chunk)` on that handler for each parallel chunk; when `Seek` is a no-op, every chunk lands at position 0 and the pipe reader sees only the last chunk. Downloads silently returned ~0 bytes.

New shim uses `*sys.MemFile` (which the SDK's download path natively detects at `downloadworker.go:492` and pre-allocates via `InitBuffer`, then uses the `io.WriterAt` parallel path at `downloadworker.go:526`). A `StatusCallback` signals completion; result returned as `bytes.NewReader(memFile.Buffer)`. Zero disk I/O.

### 2. `rclone-zus backend/zus/zus.go` — RWMutex wallet fast path

`walletMu` was `sync.Mutex`. Every `Put`/`Open`/`Update` took `Lock()`. With `--transfers=16`, all 16 goroutines serialized on this lock even though they were all using the same wallet. Upload throughput was bounded at ~6 MB/s (one file at a time).

Fix: `sync.RWMutex`. Common path (same wallet across all ops) takes `RLock()` and runs concurrently. Only a cross-wallet switch takes the write lock and blocks in-flight ops. For rclone-zus's typical single-remote workflow this is effectively lock-free.

### 3. `gosdk zcncore/wallet_base.go` + `core/client/set.go` — API shims

rclone-zus expects `SetGeneralWalletInfo(json, scheme)` and the `zboxcore/client` package with `InitSDK`. On this branch, the first call signature matched but the second didn't; and `SetWalletInfo` only populates `zcncore` state, not `zboxcore/client` state (which is where `X-App-Client-ID` is read from for blobber requests). Without this, every blobber request returned `Client id is required`.

Shim: `SetGeneralWalletInfo` forwards to `SetWalletInfo` **and** `zboxClient.PopulateClient`. New `core/client/set.go` provides `InitSDK` matching the rclone-zus expected signature.

## Compatibility

All changes are additive (new functions, new fields, new types) except the rclone-zus `walletMu` type change from `Mutex` → `RWMutex`. No existing gosdk consumer depends on the lock being exclusive; the semantic change is "multiple ops on same wallet can proceed concurrently", which is always safe for read operations and safe for SDK write operations (the SDK is already goroutine-safe for concurrent calls on one `Allocation`).

The `zboxcore/sdk/reader.go` `StreamDownload` path was intentionally **not** modified. It has independent pre-existing bugs (panics on empty `downloadQueue`, corrupt output after the first 512 bytes) but is not used by our `DownloadObject` shim and is not called from any internal gosdk consumer on this branch. If a consumer of `GetAllocationFileReader`/`DownloadFromReader` needs the streaming reader fixed in the future, the changes are known; they are not applied here to keep this branch's diff minimal.

## Reproducing the benchmark

```bash
export ZUS_ALLOC=<your allocation>
export ZUS_WALLET=/root/.zcn/wallet.json
export BLOCK_WORKER=http://198.18.0.100:9091
export RCLONE_ZUS=/root/Code/rclone_zus/rclone
./upload_compare.sh uploadcmp
cat /tmp/upload_compare_results/summary.csv
```

The script is in the `system_test` repo at `scripts/upload_compare.sh`.

## Same bug present on other branches

The `downloadQueue`-not-initialized bug in `GetDStorageFileReader` (see "not modified" note above) is present on `fix/lfb-aware-sharder-selection` and `staging` as well. Any consumer of `StreamDownload.Read` on those branches will panic. Port the one-liner `sd.downloadQueue = make(downloadQueue, len(alloc.Blobbers))` init if/when you need streaming read on those branches — but only bundled with a Read-loop fix since the queue init alone exposes a second bug (data shard concatenation) that returns corrupted bytes.
