# rclone-zus for eblobber + gosdk feat/enterprise-blobber

This branch (feature/static-builds-eblobber-compat) extends
feature/static-builds-and-rclone-compat so rclone-zus can talk to
enterprise-blobber (eblobber) backends that use the older gosdk API.

## How to build

Requires local gosdk checkout at ../gosdk on branch feat/enterprise-blobber,
with the shim commits (core/client, SetGeneralWalletInfo, PreservePath,
DownloadObject, RegisterZauthServer) applied.

    cd rclone_zus
    # uncomment the replace directive in go.mod (active on this branch for local gosdk)
    go build -tags bn256 -o rclone .

## Why

Upstream gosdk v1.20.9 (rclone default) calls /v2/connection/commit/
which eblobber does not expose. gosdk feat/enterprise-blobber uses /v1/
matching eblobber but lacks the newer core/client package and a few
zcncore symbols rclone-zus depends on.

The feat/enterprise-blobber gosdk branch has small shims added:
- core/client (InitSDK, GetClient)
- SetGeneralWalletInfo (now populates both zcncore AND zboxcore/client
  state — fixes "Client id is required" runtime error)
- RegisterZauthServer (no-op)
- sdk.OperationRequest.PreservePath (unused field)
- Allocation.DownloadObject (wraps DownloadFileToFileHandler)

This branch is compatible. Tested against eblobber with allocation
69aa58c5... on test2.zus.network.
