# 🚀 TgDrive

Encrypted cloud storage engine built in **Go** and **React / TypeScript**, using private **Telegram channels** as an infinite ciphertext storage backend, paired with **Turso (libSQL) / SQLite** for metadata indexing.

---

## 📐 Architecture & Data Flow

```
[ Web Browser ]
      │  HTTPS (TLS 1.3)
      ▼
┌─────────────────────────────────────────────────────────────────┐
│                          Go API Server                          │
│                                                                 │
│  ├── Auth: Admin JWT Token Validation                           │
│  ├── Upload: Slices stream into CHUNK_SIZE blocks (default 16MB)│
│  │   └── Encrypts each: AES-256-GCM + PBKDF2 (10,000 iters)     │
│  ├── Stream: HTTP Range (206 Partial Content)                   │
│  │   └── Paces decrypted bytes in 256 KiB blocks + client cancel│
│  ├── Memory: Bounded LRU cache + memory budget admission gate   │
│  └── Bot Pool: Round-robin dispatch across active Telegram bots │
└────────────────────────┬────────────────────────┬───────────────┘
                         │                        │
                         ▼                        ▼
           ┌──────────────────────────┐  ┌──────────────────────────┐
           │      Turso / SQLite      │  │    Telegram Bot API      │
           │      (Metadata DB)       │  │  (Encrypted Channel)     │
           │                          │  │                          │
           │ - Files & chunk hashes   │  │ - Opaque ciphertext blobs│
           │ - Encrypted bot tokens   │  │ - Unique IV per chunk    │
           │ - Upload session states  │  │ - Zero plaintext stored  │
           └──────────────────────────┘  └──────────────────────────┘
```

---

## ⚡ Technical Highlights

* **Authenticated Chunk Encryption:** Files are sliced into fixed-size blocks (`CHUNK_SIZE`, default 16 MiB) and encrypted with **AES-256-GCM (AEAD)** before dispatch to Telegram. Each chunk receives a fresh 12-byte cryptographically random IV and a 16-byte authentication tag. Key derivation uses **PBKDF2-HMAC-SHA256** (10,000 iterations) with a unique 16-byte random salt per file.
* **Token Encryption at Rest:** Telegram bot tokens stored in the database are encrypted with AES-256-GCM using `BOT_CREDENTIAL_SECRET` and indexed via SHA-256 fingerprints. Raw tokens are never logged or returned to the client.
* **Multi-Bot Worker Pool:** Multiple bot tokens (`TG_BOT_TOKENS`) run concurrently in a thread-safe pool. Transfers are balanced across bots to avoid Telegram rate limits. If a bot is revoked, chunks can be migrated live to another active bot via `POST /api/bots/{hash}/migrate`.
* **Resumable Chunked Upload Protocol:** Files are uploaded via an initialization-and-chunk lifecycle (`/upload/init` ➔ `/chunks/{idx}` ➔ `/finalize`). Each chunk verifies pre-encryption SHA-256 integrity. If interrupted, uploads resume from the last confirmed chunk.
* **Low-Latency HTTP Range Streaming:** Supports HTTP `206 Partial Content` for video and audio playback. Decrypted chunks are delivered in **256 KiB paced writes** with real-time `isClientGone` checks, enabling instant seek cancellations without socket stalls.
* **True Telegram Deletion:** Deleting a file or folder calls Telegram's `deleteMessage` API for every stored chunk before removing database records, preventing orphaned messages in the storage channel.
* **Dual Database Backend:** Runs on **Turso Cloud (libSQL)** for durable production deployments or local **SQLite** for zero-dependency development.

---

## 🛠️ Configuration Reference

Configure these environment variables in your `.env` file or deployment dashboard:

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `TG_BOT_TOKEN` | String | *Required* | Primary Telegram bot token from [@BotFather](https://t.me/BotFather). |
| `TG_BOT_TOKENS` | String | *Optional* | Comma-separated list of additional bot tokens for multi-bot parallel pooling. |
| `TG_CHANNEL_ID` | Int64 | *Required* | Numeric ID of the private Telegram storage channel (`-100...`). |
| `ENCRYPTION_SECRET` | String | *Required* | Master key for deriving per-file AES-256-GCM encryption keys (min 32 chars). |
| `BOT_CREDENTIAL_SECRET` | String | *Required in Prod* | Secret used to encrypt stored bot tokens at rest (min 32 chars). |
| `JWT_SECRET` | String | *Optional* | Secret used for signing admin JWT authentication tokens. |
| `ADMIN_PASSWORD` | String | *Required* | Dashboard login password for single-user admin access. |
| `TURSO_DATABASE_URL` | String | *Optional* | Remote Turso database URL (`libsql://...`). Recommended for production. |
| `TURSO_AUTH_TOKEN` | String | *Optional* | Authentication token for Turso cloud database. |
| `DATABASE_PATH` | String | `tgdrive.db` | Local SQLite database file path (used when Turso is not configured). |
| `CHUNK_SIZE` | Integer | `16777216` (16MB) | Raw chunk size in bytes. Set to `8388608` (8MB) or `4194304` (4MB) for lower video startup latency. |
| `CACHE_MAX_BYTES` | Integer | `67108864` (64MB) | Memory limit for decrypted in-memory LRU chunk cache. |
| `MAX_CONCURRENT_UPLOADS` | Integer | `4` | Maximum parallel upload chunk operations server-wide. |
| `MAX_CONCURRENT_DOWNLOADS` | Integer | `4` | Maximum simultaneous download/stream operations server-wide. |
| `MAX_MEMORY_BUDGET_BYTES` | Integer | `402653184` (384MB)| Server memory admission ceiling to prevent container OOMs. |
| `PORT` | Integer | `8080` | Port the Go server listens on. |

---

## 🚀 Quick Start

### 1. Telegram Setup
1. Create a bot using [@BotFather](https://t.me/BotFather) and obtain the HTTP API token.
2. Create a private Telegram channel.
3. Add your bot as an **Administrator** with **Post Messages** and **Delete Messages** permissions.
4. Forward a message from your channel to [@userinfobot](https://t.me/userinfobot) to get your numeric Channel ID (`-100...`).

### 2. Local Run

```bash
# Clone and setup env
git clone https://github.com/mrmastrodotin/TgDrive.git
cd TgDrive
cp .env.example .env
# Edit .env with your credentials

# Run Backend (Go 1.22+)
cd backend
go run main.go

# Run Frontend (Node.js 18+)
cd ../frontend
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) and sign in using your `ADMIN_PASSWORD`.

### 3. Docker Deployment

```bash
docker build -t tgdrive .
docker run -d \
  -p 8080:8080 \
  -e TG_BOT_TOKEN="your_bot_token" \
  -e TG_CHANNEL_ID="-1001234567890" \
  -e ENCRYPTION_SECRET="your_32_char_secret" \
  -e BOT_CREDENTIAL_SECRET="your_32_char_bot_secret" \
  -e ADMIN_PASSWORD="your_password" \
  -v tgdrive_data:/app/data \
  --name tgdrive \
  tgdrive
```

---

## 📡 API Reference

All protected endpoints require an `Authorization: Bearer <token>` header (or valid session cookie / stream ticket).

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `POST` | `/api/auth/login` | Authenticate with `ADMIN_PASSWORD` and get JWT. |
| `GET` | `/api/files` | List files and folders with metadata. |
| `POST` | `/api/files/upload/init` | Initialize a chunked upload session. |
| `PUT` | `/api/files/upload/{id}/chunks/{idx}` | Upload an encrypted chunk block. |
| `GET` | `/api/files/upload/{id}/status` | Check confirmed chunks and resume state. |
| `POST` | `/api/files/upload/{id}/finalize` | Assemble uploaded chunks into a completed file. |
| `POST` | `/api/files/upload/{id}/cancel` | Cancel upload session and delete partial Telegram chunks. |
| `GET` | `/api/files/{id}/stream` | Stream audio/video with HTTP Range support (`206 Partial Content`). |
| `GET` | `/api/files/{id}/thumbnail` | Retrieve decrypted thumbnail image. |
| `DELETE` | `/api/files/{id}` | Delete file from database and purge messages from Telegram. |
| `GET` | `/api/folders` | List directory structure. |
| `POST` | `/api/folders` | Create a new folder. |
| `DELETE` | `/api/folders/{id}` | Recursively delete folder and all enclosed Telegram chunks. |
| `GET` | `/api/bots` | List active bot workers and health status. |
| `POST` | `/api/bots` | Add a new bot token to the active pool. |
| `DELETE` | `/api/bots/{hash}` | Remove a bot from the pool. |
| `POST` | `/api/bots/{hash}/migrate` | Live-migrate chunks from one bot to another. |
| `GET` | `/api/stats` | Storage usage and file counts. |
| `GET` | `/api/system/memory` | Real-time container RSS, Go heap, and cache statistics. |

---

## 📄 License

MIT License. See [LICENSE](LICENSE) for details.
