# AGENTS.md — configure

> Compact context for OpenCode sessions working in this repo.

## Project

A small Go HTTP server that serves sensitive configuration files over the network with IP whitelist validation and optional AES encryption. It now also supports per-IP JSON config customization and Redis pub/sub for config change notifications.

## Architecture

- **Language**: Go 1.21.
- **Dependencies**: `github.com/redis/go-redis/v9`, `github.com/fsnotify/fsnotify` (standard library for everything else).
- **Entry point**: `main.go` starts an HTTP server on `:6001`.
- **Client example**: `client/main.go` demonstrates fetching and decrypting a remote config file.
- **Encryption utilities**: `common/encryption.go` — AES-CTR mode (random IV prepended to ciphertext) + RSA key generation helpers (unused).
- **Config management**: `common/config.go` — loads `conf/base.json` and merges per-IP overrides from `conf/overrides.json`.
- **Redis utilities**: `common/redis.go` — connection and pub/sub helper.

## Key Files & Layout

| Path | Role |
|------|------|
| `main.go` | HTTP server, whitelist check, `/configFile` (raw files), `/customConfig` (merged JSON), file watcher |
| `client/main.go` | Example consumer that GETs `config.toml` and decrypts it |
| `common/encryption.go` | AES-CTR encrypt/decrypt with random IV, RSA keygen helpers |
| `common/config.go` | JSON config manager with deep-merge for nested overrides |
| `common/redis.go` | Redis client init and publish helper |
| `whitelist.txt` | Newline-separated IPs / domains allowed to request files |
| `conf/` | Config files: `config.toml` (raw), `base.json` (JSON base), `overrides.json` (per-IP diffs) |
| `cert/` | Optional TLS cert/key files (`cert.pem`, `cert.key`) for HTTPS |
| `Dockerfile` / `docker-compose.yml` | Deployment artifacts |

## Build & Run

```bash
# Local development (no TLS, no AES)
go build -o configure .
./configure

# With AES encryption
AES_KEY=your16byteKeyHere ./configure

# With Redis pub/sub
REDIS_ADDR=127.0.0.1:6379 REDIS_PASSWORD=secret AES_KEY=your16byteKeyHere ./configure

# Docker Compose
docker-compose up --build
```

## Environment & Configuration

- `AES_KEY` — optional. If provided, must be exactly **16, 24, or 32 bytes**. Invalid lengths are logged as a warning and ignored. Server encrypts responses; client must use the same key.
- `REDIS_ADDR` — optional, defaults to `127.0.0.1:6379`.
- `REDIS_PASSWORD` — optional Redis password.
- `whitelist.txt` — read on every request (with mtime caching). Supports IPv4 and domain names. Domains are resolved to IPs and appended to the in-memory list.
- HTTPS is **auto-enabled** if `cert/cert.pem` and `cert/cert.key` exist; otherwise plain HTTP.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/configFile?fileName=xxx` | GET | Returns raw file from `conf/` (AES encrypted if key is set). |
| `/customConfig` | GET | Returns merged JSON config for the caller's IP. Applies per-IP overrides from `conf/overrides.json`. AES encrypted if key is set. |
| `/customConfig?config=sub1` | GET | Returns the merged config for `conf/sub1.json`. If `sub1.json` declares `"__extends": "base.json"`, the result is base merged with sub1's overrides. |

## Config Management

- `conf/base.json` — base JSON configuration.
- `conf/overrides.json` — map of IP → partial JSON object. Nested objects are **deep-merged** (e.g., `ServerTCP.Port` can be overridden without replacing the whole `ServerTCP` object).
- **Config inheritance** — any `.json` file in `conf/` can use `"__extends": "base.json"` to inherit from another config. The child config is deep-merged on top of the parent, so only overridden keys need to be specified. Supports multi-level inheritance (e.g., `sub2.json` extends `sub1.json` which extends `base.json`). Circular references are detected and rejected.
- The server watches `conf/` via `fsnotify`. When any `.json` file changes, the config is reloaded and a message is published to the Redis channel `config_updates`.

## Security Behavior

- **AES-CTR mode**: A random IV is generated per encryption and prepended to the ciphertext. The client must extract the first 16 bytes as the IV before decrypting.
- **RSA helpers**: Present in `common/encryption.go` but unused in the server path.
- **Client example**: `client/main.go` embeds a hardcoded AES key — it is an example, not a production secret manager.

## Testing & Quality

- **No tests exist** — no `*_test.go` files, no test framework, no CI workflows.
- **No lint / format scripts** — rely on standard `go fmt`, `go vet`, and `go build`.
- To verify changes, run the server and hit the endpoints:
  ```bash
  curl "http://127.0.0.1:6001/configFile?fileName=config.toml"
  curl "http://127.0.0.1:6001/customConfig"
  ```

## Common Pitfalls

1. **Whitelist mismatch**: `request.RemoteAddr` is parsed with `net.SplitHostPort`, which correctly handles IPv4 and IPv6. IPv6 localhost (`[::1]`) is supported.
2. **Missing `conf/` or `whitelist.txt`**: Missing files now return `404` or `500` with proper HTTP status codes instead of silent empty responses.
3. **AES key length mismatch**: Invalid key lengths are logged as a warning and the server falls back to plaintext.
4. **Config reload race**: The file watcher reloads asynchronously. If you curl immediately after editing a config file, you may see the old content. Wait a moment for the watcher to process the event.

## References

- `README.md` — high-level usage overview in Chinese.
- `docker-compose.yml` — exact volume paths and environment variable expectations.
