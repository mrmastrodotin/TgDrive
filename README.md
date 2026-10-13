# 🚀 TgDrive — Encrypted Cloud Storage Powered by Telegram & Go

TgDrive is a self-hosted personal cloud storage platform that uses Telegram channels as an encrypted file storage backend, paired with an SQLite/Turso metadata database and a Go API server.

---

## 🔒 Security & Architecture

```
[Web Browser / Mobile PWA]
          │
          │ HTTP / Chunked Stream
          ▼
┌─────────────────────────────────────────┐
│              Go Backend                 │
│  - AES-256-GCM storage encryption       │
│  - 16MB chunk pipeline (CHUNK_SIZE)     │
│  - PBKDF2 Key derivation per file       │
│  - User-owned Telegram bot worker pools │
│  - Range header byte seeking            │
└──────────────────┬──────────────────────┘
                   │
         ┌─────────┴─────────┐
         │                   │ Encrypted chunks only
         ▼                   ▼
┌──────────────────┐  ┌──────────────────────┐
│  SQLite / Turso  │  │ Telegram Bot API     │
│  (Metadata DB)   │  │ (Encrypted Channel)  │
└──────────────────┘  └──────────────────────┘
```

- **Server-Side Authenticated Storage Encryption:** Plaintext files are transferred over TLS/HTTPS, split into 16MB blocks (`CHUNK_SIZE`), and encrypted using AES-256-GCM before transmitting to Telegram. Telegram never receives unencrypted file names, raw data, or keys.
- **Dedicated Bot Credential Encryption at Rest:** Telegram bot tokens are encrypted at rest with AES-256-GCM using a dedicated `BOT_CREDENTIAL_SECRET` and indexed via cryptographic SHA-256 fingerprints. Raw tokens are never returned to clients or logged.
- **Bot Worker Pools & Rotation:** Supports multiple bot tokens for parallel transfers. In-place token rotation preserves existing chunks, while distinct-bot replacements retain original credentials as download-only workers or migrate chunks via resumable ciphertext transfer (`POST /api/bots/{hash}/migrate`).
- **Deployment Identity Model:** The production web deployment operates in **Single-User Administrator Mode** authenticated via `ADMIN_PASSWORD` (issuing administrative JWTs). While database schemas and bot pools enforce `owner_id` multi-tenant boundaries for architectural extensibility, self-service multi-user accounts are not exposed in the standalone production web UI.
- **Deterministic Memory Budget & Admission Control:** Operations enforce resource-aware admission control against `MAX_MEMORY_BUDGET_BYTES`. Real-time Linux container RSS, cgroups (v1/v2), Go heap, and admission counters are exposed at `GET /api/system/memory`.
- **HTTP Range Streaming:** Stream movies, audio, and large PDFs with random seek support directly in browser without downloading the full file first.
- **Multi-File Sequential Uploads:** Upload entire folder structures with progress bars.

---

## 🛠️ Prerequisites

1. **Telegram Bot Token:**
   - Talk to [@BotFather](https://t.me/BotFather) on Telegram.
   - Create a new bot with `/newbot`.
   - Copy the HTTP API token.

2. **Telegram Storage Channel:**
   - Create a new private channel in Telegram.
   - Add your bot as an Administrator with **Post Messages** permission.
   - Send any message to the channel, then forward it to [@userinfobot](https://t.me/userinfobot) to get your numeric Channel ID (usually starts with `-100`).

3. **Go (1.22+)** & **Node.js (18+)** for local development.

---

## ⚡ Quick Start (Local Development)

### 1. Configure Environment Variables
Copy `.env.example` to `.env` in the project root:

```bash
cp .env.example .env
```

Edit `.env` with your credentials:
```env
TG_BOT_TOKEN=your_bot_token_from_botfather
TG_CHANNEL_ID=-1001234567890
ENCRYPTION_SECRET=your_super_strong_random_secret_at_least_32_characters
BOT_CREDENTIAL_SECRET=dedicated_secret_for_bot_credentials_at_least_32_characters
ADMIN_PASSWORD=your_dashboard_password
PORT=8080
DATABASE_PATH=tgdrive.db
```

### 2. Run Backend
```bash
cd backend
go run main.go
```

### 3. Run Frontend
In a new terminal:
```bash
cd frontend
npm install
npm run dev
```
Open [http://localhost:3000](http://localhost:3000) in your browser.

---

## 🐳 Docker Deployment

To build and run the production container:

```bash
docker build -t tgdrive .
docker run -d \
  -p 8080:8080 \
  -e TG_BOT_TOKEN="your_token" \
  -e TG_CHANNEL_ID="-1001234567890" \
  -e ENCRYPTION_SECRET="your_secret" \
  -e BOT_CREDENTIAL_SECRET="your_bot_secret" \
  -e ADMIN_PASSWORD="your_password" \
  -v tgdrive_data:/app/data \
  --name tgdrive-app \
  tgdrive
```

---

## ☁️ 1-Click Deployment (Render)

This repository includes a `render.yaml` configuration blueprint.
1. Push this repository to GitHub.
2. Link your GitHub repo to [Render](https://render.com).
3. Set the environment variables in the Render dashboard:
   - `TG_BOT_TOKEN`
   - `TG_CHANNEL_ID`
   - `ENCRYPTION_SECRET`
   - `BOT_CREDENTIAL_SECRET`
   - `ADMIN_PASSWORD`
4. Render automatically compiles the multi-stage Docker build and spins up the live service.
