# TgDrive Encryption Architecture & Threat Model

This document specifies the actual encryption architecture implemented in TgDrive, clarifies cryptographic claims, and outlines the engineering roadmap required to achieve genuine client-side zero-knowledge encryption.

---

## 1. Current Architecture: Server-Side Authenticated Storage Encryption

TgDrive currently employs **Server-Side Authenticated Storage Encryption** backed by **AES-256-GCM** and **PBKDF2**.

```
[Client Browser]
       │
       │ Plaintext chunks over TLS 1.3 / HTTPS
       ▼
[Go Backend Server]
   ├── Receives raw bytes into bounded RAM buffers
   ├── Derives file key: PBKDF2(ENCRYPTION_SECRET, file.salt, 10,000 iterations, 32 bytes)
   ├── Generates random 12-byte IV via crypto/rand
   ├── Encrypts chunk: AES-256-GCM (Ciphertext + 16-byte Poly1305/GHASH Auth Tag)
   ├── Drops raw plaintext buffer from memory immediately (rawData = nil)
   ▼
[Telegram Bot API / Private Storage Channel]
   └── Receives only authenticated ciphertext blobs
```

### Cryptographic Primitives

| Component | Primitive | Parameters |
| :--- | :--- | :--- |
| **Transport** | TLS 1.2 / TLS 1.3 | Ephemeral ECDHE key exchange, modern cipher suites |
| **Cipher** | AES-256-GCM | Authenticated Encryption with Associated Data (AEAD) |
| **Authentication Tag** | GHASH (GCM Tag) | 128-bit (16 bytes) appended to each chunk |
| **Key Derivation** | PBKDF2-HMAC-SHA256 | 10,000 iterations, 32-byte derived key |
| **Salt** | Cryptographically Random | 16 bytes generated per file via `crypto/rand` |
| **Nonce / IV** | Unique Random IV | 12 bytes (96 bits) generated per chunk via `crypto/rand` |
| **Chunk Size** | Standardized `CHUNK_SIZE` | 16 MiB default (comfortably under Telegram 20 MiB ceiling) |

### Confidentiality & Integrity Properties

1. **Telegram Zero-Access**: Telegram servers and network observers never observe raw file contents, filenames, or metadata. All chunks posted to Telegram channels are opaque AES-256-GCM ciphertext blobs.
2. **Cryptographic Authentication**: Every chunk is cryptographically signed by AES-256-GCM's 128-bit authentication tag. Any tampering, byte truncation, or bit-flip by Telegram or intermediate nodes causes authentication failure and immediate request rejection.
3. **Nonce Uniqueness**: Each chunk uses a fresh 12-byte random IV. Combined with per-file salt derivation, identical plaintext chunks (e.g. padding or repeated media frames) produce completely distinct ciphertexts.

---

## 2. Distinction from Zero-Knowledge Encryption

### Why the Current Model is NOT Zero-Knowledge Encryption

- **Plaintext Access at the Server**: The browser sends plaintext data over HTTPS to the Go backend. During upload and download, the Go backend temporarily handles plaintext in volatile memory buffers.
- **Server-Managed Key Material**: The encryption key is derived using `ENCRYPTION_SECRET` configured on the backend server. The client browser does not hold an exclusive, unshared cryptographic key.
- **Server-Side Streaming Decryption**: When a client requests byte-range streaming for video or audio, the Go server downloads encrypted chunks from Telegram, decrypts them in-memory, and streams decrypted byte ranges to the client.
- **Trust Boundary**: The user must trust the Go backend server (its hosting provider, environment configuration, and runtime security).

Consequently, describing this architecture as "zero-knowledge encryption" or claiming "client-held keys" was technically inaccurate. The system is accurately described as **AES-256-GCM Server-Side Authenticated Storage Encryption**.

---

## 3. Roadmap to Genuine Client-Side Zero-Knowledge Encryption

To achieve true zero-knowledge end-to-end encryption (where the backend server cannot decrypt user files even if compelled or compromised), the following architectural changes are required:

### 3.1 Client-Side Key Derivation (Web Crypto API)
- The user inputs an account passphrase or recovery key in the browser.
- The browser uses the Web Crypto API (`crypto.subtle`) or Argon2id compiled to WebAssembly to derive a 256-bit Master Key.
- The Master Key is stored only in ephemeral browser memory (`sessionStorage` or WebCrypto non-exportable `CryptoKey` handles) and **never transmitted to the backend**.

### 3.2 Client-Side Chunking and AEAD Encryption
- During file upload, the browser slices files into 16 MiB chunks using `File.slice()`.
- Each chunk is encrypted in the browser with `crypto.subtle.encrypt({ name: "AES-GCM", iv: randomIV }, derivedKey, chunkData)`.
- The browser uploads ciphertext chunks directly to the backend. The backend acts solely as an untrusted ciphertext relay to Telegram.

### 3.3 In-Browser Streaming Decryption (Service Worker)
- For audio and video playback, a registered Service Worker intercepts range requests from `<video>` or `<audio>` elements.
- The Service Worker fetches encrypted chunks from the backend, decrypts them in-browser with `crypto.subtle.decrypt()`, and responds with `206 Partial Content` streams.
- The backend never decrypts or accesses the video stream.

### 3.4 Client-Side Thumbnails & Previews
- Thumbnails for images and video posters must be generated client-side (via Canvas / WebCodecs API) and encrypted as separate blobs before upload.
- Full-text search and metadata indexing must either use encrypted searchable indexes (e.g., SSE / ORAM) or remain client-side.

### 3.5 Key Recovery & Multi-Device Sync
- Implement BIP-39 mnemonic phrases (12 or 24 words) for passphrase recovery.
- For multi-device access, use asymmetric key exchange (e.g., X25519) to provision new devices without sharing plaintext passphrases over the network.

---

## 4. Bot Credential Authenticated Encryption at Rest

TgDrive protects stored Telegram bot worker credentials against database leaks or storage snapshots by enforcing **Authenticated Encryption at Rest**.

### 4.1 Schema & Credential Storage Format

The `registered_bots` SQLite table stores worker records without plaintext tokens:

```sql
CREATE TABLE IF NOT EXISTS registered_bots (
    token_hash TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL,
    token      TEXT NOT NULL,
    disabled   INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_registered_bots_owner ON registered_bots(owner_id);
```

- **Token Fingerprint (`token_hash`)**: Cryptographically stable SHA-256 digest (`crypto.HashToken(rawToken)`) used as the immutable primary key for worker lookup, chunk attribution, and ownership validation without exposing token material.
- **Encrypted Token (`token`)**: Encrypted using AES-256-GCM formatted with versioned header:
  ```text
  enc:v1:<12-byte hex IV>:<hex AES-GCM ciphertext + 16-byte auth tag>
  ```
- **Owner Scope (`owner_id`)**: Immutable ownership identifier. Only the owning identity can view, disable, rotate, or delete the bot worker.

### 4.2 Key Derivation & Isolation

- **Dedicated Production Secret**: `BOT_CREDENTIAL_SECRET` is required in production deployments. It provides cryptographic isolation between stored bot credentials and user file ciphertext.
- **Migration & Compatibility Fallback**: If `BOT_CREDENTIAL_SECRET` is unset in development/migration mode, the system falls back to `ENCRYPTION_SECRET` and emits a prominent startup warning:
  ```text
  ⚠️ COMPATIBILITY WARNING: BOT_CREDENTIAL_SECRET is unset. Using legacy fallback to ENCRYPTION_SECRET in migration/compatibility mode. In production, configure a dedicated BOT_CREDENTIAL_SECRET.
  ```
- **Domain Separation**: Derived via PBKDF2-HMAC-SHA256 with 100,000 iterations using the domain-separated salt `"tgdrive:bot_credential_at_rest_v1"`.
- **In-Memory Decryption**: Bot tokens are decrypted only on-demand when initializing Telegram bot client instances in the worker pool.
- **Safe Decryption Failure**: If a stored credential cannot be decrypted (e.g. wrong key, corrupted ciphertext, or unrotated secret), `decryptBotToken` returns an empty string and the backend logs an alert while safely skipping worker pool initialization, preventing crashes or ciphertext leaks.
- **Zero Frontend Leakage**: The `storage.RegisteredBot` model specifies `Token string json:"-"`. The raw token is strictly omitted from all JSON API responses and logs.

### 4.3 Secret Lifecycle & Independence

TgDrive separates operational secrets into three distinct domains:

| Secret | Purpose | Default / Fallback | Rotation Impact |
| :--- | :--- | :--- | :--- |
| `ENCRYPTION_SECRET` | Derives per-file AES-256-GCM chunk keys with file salt | Mandatory in all environments | Changing this invalidates all stored file chunks unless re-encrypted |
| `BOT_CREDENTIAL_SECRET` | Encrypts stored Telegram bot tokens at rest | Mandatory in production (`ENV=production`); dev falls back to `ENCRYPTION_SECRET` with warning | Changing requires running `db.RotateBotCredentials(ctx, oldKey, newKey)` |
| `JWT_SECRET` | Signs authentication tokens and playback tickets | Falls back to first 32 bytes of `ENCRYPTION_SECRET` | Changing immediately invalidates active sessions/tickets without impacting files or bot workers |

### 4.4 Backup, Disaster Recovery & Key Rotation Procedures

1. **Database Durability in Production**:
   - On ephemeral cloud hosts (Render, Koyeb), local SQLite databases reset on container restart or redeploy.
   - Production startup (`ENV=production` or `REQUIRE_PERSISTENT_DB=true`) **strictly requires** `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN`.
   - The server fails fast and terminates startup if `TURSO_AUTH_TOKEN` is missing or if the server attempts to fall back silently to ephemeral SQLite without an explicit `ALLOW_LOCAL_SQLITE=true` flag.
2. **Database Backup**:
   - Local SQLite: `sqlite3 /app/data/tgdrive.db ".backup '/app/data/tgdrive_backup.db'"`
   - Turso Cloud: Automated continuous point-in-time recovery via Turso, or dump via `turso db shell <db-name> .dump > backup.sql`.
3. **Restoration**:
   - Restore database from backup file or Turso branch.
   - Ensure the deployment environment variables (`BOT_CREDENTIAL_SECRET`, `ENCRYPTION_SECRET`, `JWT_SECRET`) exactly match the keys configured when the backup was created.
4. **Bot Credential Key Rotation**:
   - To change `BOT_CREDENTIAL_SECRET`, call the programmatic rotation method:
     ```go
     oldKey := crypto.DeriveCredentialKey(oldSecret)
     newKey := crypto.DeriveCredentialKey(newSecret)
     err := db.RotateBotCredentials(ctx, oldKey, newKey)
     ```
   - Once all rows in `registered_bots` are re-encrypted with `newKey`, update `BOT_CREDENTIAL_SECRET` in environment variables and restart the server.
5. **Safe Decryption Failures**:
   - If a stored bot token cannot be decrypted due to an invalid key or data corruption, `decryptBotToken` logs an error and returns an empty string. The worker pool skips the bot without crashing or leaking ciphertext.

---

## 5. Telegram Bot Rotation, Identity Replacement & Chunk Migration

### 5.1 Telegram `file_id` Boundary & Invariant

In Telegram's Bot API, `file_id` references are cryptographically and logically scoped to the specific bot identity that uploaded or received the document. **A different Telegram bot identity cannot download or query another bot's `file_id` via `getFile`** (Telegram returns `400 Bad Request: wrong remote file identifier specified`).

TgDrive strictly enforces this invariant:
- Chunks record their uploading bot's token hash in the database (`chunks.bot_token_hash`).
- `DownloadChunk` executes downloads strictly using the registered bot worker corresponding to that hash.

### 5.2 Token Rotation vs Bot Identity Replacement

TgDrive automatically inspects Telegram bot tokens (`<bot_id>:<secret>`) to distinguish between rotating a token for the same bot identity vs replacing it with a new bot identity:

1. **Same-Bot In-Place Rotation (`isSameBot == true`)**:
   - Occurs when rotating credentials with BotFather for the same bot ID (e.g. `12345:TOKEN_A` -> `12345:TOKEN_B`).
   - Telegram retains document access: all existing `file_id`s remain valid under the new token.
   - The worker client in `BotPool` is updated in-place with the new token.
   - Hash aliases (`hashAliases[oldHash] = newHash`) are established in memory so existing requests resolve immediately.
   - The database atomically updates `registered_bots` and re-indexes all stored chunks via `UpdateChunkBotTokenHash(oldHash, newHash)`.

2. **Different-Bot Identity Replacement (`isSameBot == false`)**:
   - Occurs when replacing a worker with a distinct bot identity (e.g. `12345:TOKEN_A` -> `99999:TOKEN_C`).
   - The replacement bot cannot access Bot A's `file_id`s.
   - **Credential Preservation**: The old bot (`12345`) is automatically marked `downloadOnly = true` and retained in `BotPool` and `registered_bots`. It stops taking new uploads, but continues serving download and stream requests for its existing chunks.
   - **Safeguard Enforcement**: Calling `POST /api/bots/{hash}/disable` or `DELETE /api/bots/{hash}` is rejected with `409 Conflict` if chunks still depend on that bot, preventing accidental file corruption.

### 5.3 Resumable, Idempotent Chunk Migration (`POST /api/bots/{hash}/migrate`)

To decommission a bot after identity replacement, TgDrive provides an administrative migration endpoint:

```http
POST /api/bots/{sourceHash}/migrate
Content-Type: application/json

{"target_bot_hash": "<optional target hash>"}
```

**Migration Sequence**:
1. Fetches all chunks associated with `sourceHash` from SQLite.
2. Iterates chunk by chunk:
   - **Crash Recovery & Reconciliation**: Before downloading or uploading, checks `chunk_migrations` for a prior uncommitted upload to `targetHash`. If an upload already succeeded during a previously crashed/interrupted attempt, TGDrive reconciles and commits the existing `target_file_id` and `target_msg_id` directly, avoiding duplicate uploads or accumulating orphaned Telegram messages.
   - **Download Ciphertext**: Fetches the raw encrypted ciphertext blob from Telegram using the source bot.
   - **Upload Ciphertext**: Uploads the identical ciphertext blob to Telegram using the replacement bot.
   - **Staged State Persistence**: Immediately records `(chunk_id, source_hash, target_hash, target_file_id, target_msg_id, status='uploaded')` into `chunk_migrations` in SQLite. If a crash or network timeout occurs during the subsequent DB commit, retries will reconcile rather than creating duplicate Telegram documents.
   - **Verified DB Commit**: Atomically updates `telegram_file_id`, `telegram_msg_id`, and `bot_token_hash` in `chunks` in SQLite only after upload success is confirmed.
   - **Staging Cleanup**: Deletes the `chunk_migrations` record upon confirmed DB commit.
   - **Safe Old Reference Deletion**: Deletes the source Telegram message ONLY after the destination database update is confirmed and verified. Source messages are never deleted beforehand.
3. **Idempotent & Resumable**: If interrupted by network timeouts, database blips, or container restarts, re-running the migration immediately resumes at the first un-migrated chunk, reconciling any half-finished chunks seamlessly.

---

## 6. Production User Identity & Access Control

### 6.1 Single-User Mode vs Multi-User Isolation

- **Current Production Mode**: TgDrive operates strictly in **Single-User Admin Mode** for standalone deployments. Access to `/api/auth/login` requires the configured `ADMIN_PASSWORD` (or `APP_PASSWORD`), which issues a JWT with explicit claim `Subject: "admin"`.
- **Honest Architectural Boundary**: The database schema and bot pool runtime support per-user isolation (`owner_id`), allowing future multi-user authentication providers to partition bot pools and storage. However, the current web application login flow does NOT provide self-service multi-user accounts. Tests that manually generate JWTs with distinct subjects verify the backend isolation invariants, but the production web application remains single-user administrator mode until a dedicated multi-user identity provider is configured.

### 6.2 Scoped Playback Tickets

For HTML5 video and audio streaming where browsers cannot attach `Authorization: Bearer <token>` headers:
- Short-lived tickets (5-minute TTL) are issued via `POST /api/files/{id}/ticket?action=stream`.
- **Strict Audience Scoping**: Tickets contain `Audience: ["<action>:<fileID>"]`.
- A ticket issued with `action=stream` is rejected with `403 Forbidden` if presented to `/api/files/{id}/download` or any other file/endpoint.
- **Referrer Privacy**: Media endpoints set `Referrer-Policy: no-referrer` to prevent ticket leakage in browser `Referer` headers.
- **Log Redaction**: Access and error logging utilizes `RedactSensitiveURL()` to sanitize `ticket` and `token` query parameters.

---

## 7. Atomic In-Flight Memory Accounting & Linux Container Verification

### 7.1 Container Resident Memory vs Go Virtual Space

`runtime.MemStats.Sys` records virtual memory requested by the Go runtime from the operating system, including mapped address space, scavenged pages, and heap arena reservations. It does not measure physical container Resident Set Size (RSS).

TgDrive samples Linux process and container memory metrics:
- **Process RSS**: Parsed from `/proc/self/status` (`VmRSS`) and High Water Mark (`VmHWM`).
- **Container Cgroup v2**: Parsed from `/sys/fs/cgroup/memory.current` and `/sys/fs/cgroup/memory.peak`.
- **Container Cgroup v1**: Parsed from `/sys/fs/cgroup/memory/memory.usage_in_bytes` and `memory.max_usage_in_bytes`.

### 7.2 Administrative Telemetry Endpoint (`GET /api/system/memory`)

Admin callers receive complete real-time memory and admission telemetry:

```json
{
  "process_rss_bytes": 45182976,
  "peak_process_rss_bytes": 58720256,
  "container_memory_bytes": 48234496,
  "peak_container_memory_bytes": 62914560,
  "container_memory_limit": 536870912,
  "cgroup_version": "v2",
  "cgroup_available": true,
  "rss_available": true,
  "go_alloc_bytes": 3543240,
  "go_sys_bytes": 27273036,
  "go_heap_alloc_bytes": 3543240,
  "go_heap_inuse_bytes": 5038080,
  "in_flight_bytes": 0,
  "peak_in_flight_bytes": 33554432,
  "memory_budget_bytes": 402653184,
  "rejected_requests": 0,
  "cache_entries": 1,
  "cache_used_bytes": 16777216,
  "upload_sem_active": 0,
  "upload_sem_capacity": 4,
  "download_sem_active": 0,
  "download_sem_capacity": 4
}
```

### 7.3 Atomic Admission Control & Bounded Buffers

1. **Atomic In-Flight Reservation**: Handlers call `s.ReserveMemory(estimatedBytes)` before allocating chunk buffers. It atomically accounts for in-flight buffers across concurrent goroutines (`inFlightMemoryBytes atomic.Int64`).
2. **Admission Enforcement**: If `inFlightMemoryBytes + HeapAlloc + estimatedBytes > MaxMemoryBudgetBytes`, the LRU chunk cache is cleared and garbage collection is triggered. If memory remains insufficient, requests are rejected and increment `rejected_requests`.
3. **Bounded Request Payloads**: Upload chunk endpoints enforce `http.MaxBytesReader` to strictly prevent unbounded request body buffering.
4. **Deterministic Release**: Every reservation returns an idempotent `release()` callback invoked via `defer`, ensuring reservations are immediately freed even on client disconnects or network failures.


