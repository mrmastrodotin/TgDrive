<div align="center">

# 🚀 TgDrive — Encrypted Cloud Storage Powered by Telegram & Go

**Turn private Telegram channels into your personal, unlimited, encrypted cloud storage.**

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=for-the-badge&logo=go)](https://golang.org)
[![React](https://img.shields.io/badge/React-18-61DAFB?style=for-the-badge&logo=react)](https://react.dev)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.0+-3178C6?style=for-the-badge&logo=typescript)](https://www.typescriptlang.org)
[![Vite](https://img.shields.io/badge/Vite-6.0+-646CFF?style=for-the-badge&logo=vite)](https://vitejs.dev)
[![Encryption](https://img.shields.io/badge/Security-AES--256--GCM-success?style=for-the-badge&logo=shield)](docs/ENCRYPTION_ARCHITECTURE.md)
[![Database](https://img.shields.io/badge/Database-Turso%20%7C%20SQLite-00E599?style=for-the-badge&logo=sqlite)](https://turso.tech)
[![License](https://img.shields.io/badge/License-MIT-blue?style=for-the-badge)](LICENSE)

[Features](#-key-features) • [Architecture](#-architecture--data-flow) • [Quick Start](#-quick-start) • [Deployment](#-deployment-options) • [Configuration](#-configuration-reference) • [Performance Tuning](#-streaming--performance-tuning) • [API Reference](#-api-overview)

</div>

---

## 📖 Overview

**TgDrive** is an open-source, self-hosted personal cloud storage platform. It harnesses the infinite document storage capacity of Telegram private channels while maintaining **strict privacy and security** through server-side AES-256-GCM authenticated encryption.

Paired with a high-performance **Go backend**, an **embedded SQLite or cloud Turso (libSQL) metadata database**, and a modern **React + Vite glassmorphic web dashboard**, TgDrive delivers a Google Drive-like experience with smooth video streaming, instant thumbnail previews, multi-bot pooling, and multi-gigabyte resumable uploads.

---

## ✨ Key Features

### 🔒 Cryptographic Privacy & Zero Telegram Leaks
* **AES-256-GCM Authenticated Encryption:** Every file chunk is encrypted using authenticated AEAD before being sent to Telegram. Telegram servers only ever receive opaque, pseudorandom ciphertext blobs.
* **Per-File PBKDF2 Key Derivation:** Every file generates a unique cryptographically secure 16-byte salt; keys are derived using PBKDF2-HMAC-SHA256 (10,000 iterations). Identical files yield completely different ciphertexts.
* **Bot Credentials Encrypted at Rest:** Bot tokens are protected by a dedicated `BOT_CREDENTIAL_SECRET` in the database and indexed with SHA-256 fingerprints. Raw tokens are never logged or exposed to the client.
* **Stored XSS & CSP Defense:** Strict `sandbox; default-src 'none'` Content Security Policy headers applied to preview streams; SVG, HTML, and XML uploads are sanitized and rendered as safe plaintext.

### 🧩 Client-Side Chunking & Resumable Uploads
* **Browser-Native Chunk Slicing:** The browser slices large files into zero-copy Blobs (`file.slice(start, end)`) matching the server's configured `CHUNK_SIZE` (default 16MB).
* **Chunked Upload Session Protocol:** Multi-gigabyte files are uploaded chunk-by-chunk with automatic resume from the last confirmed chunk, local session recovery (`localStorage`), and SHA-256 integrity verification.
* **True Telegram Deletions:** Deleting a file or folder in TgDrive calls Telegram's `deleteMessage` API for every stored chunk before removing database records, keeping your Telegram storage channel clean.

### ⚡ Multi-Bot Parallel Worker Pool
* **Parallel Transfer Concurrency:** Connect multiple Telegram bots (`TG_BOT_TOKENS`) to automatically distribute chunk uploads and downloads, eliminating Telegram rate limits.
* **In-Place Bot Rotation & Live Migration:** Rotate bot tokens or migrate file chunks across bots using resumable ciphertext copy (`POST /api/bots/{hash}/migrate`) without re-downloading files to your local machine.
* **Universal Download Fallback (`DownloadFileAnyBot`):** Files remain fully downloadable even if specific bots are disabled or legacy chunks lack token fingerprints.

### 🎬 YouTube-Grade Video Streaming & Cinema Player
* **HTTP Range Byte Seeking (`206 Partial Content`):** Stream 4K/1080p videos with random timeline seeking directly inside the browser.
* **256 KiB Paced Packet Streaming:** Decrypted bytes are flushed in smooth 256 KiB blocks with sub-millisecond disconnect detection (`isClientGone`). Scrubbing and seeking cancel previous network tasks instantly.
* **Full Keyboard Shortcuts:**
  * <kbd>Space</kbd> / <kbd>K</kbd>: Play / Pause
  * <kbd>J</kbd> / <kbd>L</kbd> or <kbd>←</kbd> / <kbd>→</kbd>: Skip backward/forward 10s
  * <kbd>0</kbd>–<kbd>9</kbd>: Jump to 0% – 90% of duration
  * <kbd>F</kbd>: Fullscreen | <kbd>T</kbd>: Theater Mode | <kbd>I</kbd> / <kbd>P</kbd>: Picture-in-Picture
  * <kbd>,</kbd> / <kbd>.</kbd>: Frame-by-frame step
  * <kbd>M</kbd>: Mute/Unmute | <kbd>↑</kbd> / <kbd>↓</kbd>: Volume control
* **Stats for Nerds & Buffer Health:** Real-time diagnostics card reporting resolution, buffer health ahead of playhead, prefetch engine status, and encryption status.
* **Subtitles & Cinema Mode:** Load external `.srt` / `.vtt` subtitle files (with automatic WebVTT conversion), and pin favorite videos to Cinema mode.

---

## 🏗️ Architecture & Data Flow

```
                               ┌────────────────────────────────────────────────────────┐
                               │                    Client Browser                      │
                               │                (React 18 + Vite + TS)                  │
                               │                                                        │
                               │  1. Init: Sends metadata ➔ POST /api/files/upload/init  │
                               │  2. Slicing: Slices file via `file.slice(start, end)`  │
                               │     into zero-copy CHUNK_SIZE Blobs (e.g. 16MB)        │
                               │  3. Upload: Concurrently streams chunks to backend     │
                               │     ➔ PUT /api/files/upload/{id}/chunks/{idx}          │
                               └───────────────────────────┬────────────────────────────┘
                                                           │ HTTPS (TLS 1.3)
                                                           ▼
┌────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│                                           Go Backend Server                                            │
│                                                                                                        │
│   ┌──────────────────────────┐    ┌───────────────────────────┐    ┌───────────────────────────────┐   │
│   │    Auth & Middleware     │    │   Encryption & Streaming  │    │     Memory Budget Manager     │   │
│   │  - Single-user Admin JWT │    │  - AES-256-GCM AEAD       │    │  - Heap Admission Gate (384MB)│   │
│   │  - Scoped Stream Tokens  │    │  - PBKDF2 (10,000 iters)  │    │  - LRU Decrypted Cache (64MB) │   │
│   │  - Stored XSS Sandbox CSP│    │  - 256 KiB Paced Streaming│    │  - Global Concurrency Limits  │   │
│   └──────────────────────────┘    └───────────────────────────┘    └───────────────────────────────┘   │
└─────────────────────────────────┬─────────────────────────────────┬────────────────────────────────────┘
                                  │                                 │
                                  ▼                                 ▼
                    ┌───────────────────────────┐     ┌───────────────────────────┐
                    │    Turso Cloud / SQLite   │     │    Telegram Bot API       │
                    │       (Metadata DB)       │     │  (Encrypted Storage Chan) │
                    │                           │     │                           │
                    │ - File & chunk index rows │     │ - Multi-bot worker pool   │
                    │ - Pre-encryption SHA-256  │     │ - Opaque ciphertext blobs │
                    │ - Encrypted bot tokens    │     │ - Unique 12B IV per chunk │
                    │ - Upload session states   │     │ - Zero plaintext stored   │
                    └───────────────────────────┘     └───────────────────────────┘
```

---

## ⚡ Quick Start

### 1. Prerequisites
1. **Telegram Bot Token:**
   * Chat with [@BotFather](https://t.me/BotFather) on Telegram.
   * Send `/newbot`, name your bot, and copy the API token.
2. **Telegram Storage Channel:**
   * Create a private channel in Telegram.
   * Add your bot as an **Administrator** with **Post Messages** and **Delete Messages** permissions.
   * Send a test message in the channel and forward it to [@userinfobot](https://t.me/userinfobot) to get your numeric Channel ID (e.g. `-1001234567890`).

### 2. Local Environment Setup
Clone the repository and copy the environment template:
```bash
git clone https://github.com/mrmastrodotin/TgDrive.git
cd TgDrive
cp .env.example .env
```

Configure your `.env` file:
```env
# Telegram
TG_BOT_TOKEN=123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ
TG_CHANNEL_ID=-1001234567890

# Secrets (Generate at least 32 random characters each)
ENCRYPTION_SECRET=your_super_secure_file_encryption_secret_min_32_chars
BOT_CREDENTIAL_SECRET=your_dedicated_secret_for_storing_bot_tokens_min_32_chars
JWT_SECRET=your_dedicated_jwt_token_signing_secret_min_32_chars

# Admin Login
ADMIN_PASSWORD=your_secure_dashboard_password

# Database (Local SQLite or Turso)
DATABASE_PATH=tgdrive.db

# Server
PORT=8080
ALLOWED_ORIGIN=http://localhost:3000
```

### 3. Run Locally

**Run Backend (Go 1.22+):**
```bash
cd backend
go run main.go
```

**Run Frontend (Node.js 18+):**
```bash
cd frontend
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000) and log in with your `ADMIN_PASSWORD`.

---

## 🚀 Deployment Options

### Option A: 1-Click Render Deployment (Recommended)

TgDrive includes a native [`render.yaml`](file:///w:/work%20space/projects/TgDrive/render.yaml) blueprint.

1. Fork or push this repository to your GitHub account.
2. In the [Render Dashboard](https://dashboard.render.com), click **New** ➔ **Blueprint** and connect your repository.
3. Configure your environment variables in Render:
   * `TG_BOT_TOKEN`
   * `TG_CHANNEL_ID`
   * `ENCRYPTION_SECRET`
   * `BOT_CREDENTIAL_SECRET`
   * `ADMIN_PASSWORD`
   * `TURSO_DATABASE_URL` (e.g., `libsql://your-db.turso.io`)
   * `TURSO_AUTH_TOKEN` (your Turso access token)
4. Render automatically compiles the multi-stage Docker build and provides a live HTTPS URL (e.g. `https://tgdrive.onrender.com`).

---

### Option B: Docker / Docker Compose

**Using Docker Compose:**
```bash
docker compose up -d
```

**Using Standalone Docker Container:**
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

## ⚙️ Configuration Reference

| Variable | Default | Description |
| :--- | :--- | :--- |
| `TG_BOT_TOKEN` | *Required* | Primary Telegram bot token from `@BotFather`. |
| `TG_BOT_TOKENS` | *Optional* | Comma-separated list of multiple bot tokens for parallel pool transfers. |
| `TG_CHANNEL_ID` | *Required* | Numeric ID of the private Telegram storage channel (`-100...`). |
| `ENCRYPTION_SECRET` | *Required* | Master key used for PBKDF2 file encryption (min 32 chars). |
| `BOT_CREDENTIAL_SECRET` | *Required in Prod* | Dedicated secret for AES-256-GCM encryption of bot tokens at rest. |
| `JWT_SECRET` | *Optional* | Dedicated signing secret for authentication tokens (defaults to `ENCRYPTION_SECRET`). |
| `ADMIN_PASSWORD` | *Required* | Password required to access the web dashboard. |
| `TURSO_DATABASE_URL` | *Optional* | Remote Turso cloud database connection (`libsql://...`). Recommended for production durability. |
| `TURSO_AUTH_TOKEN` | *Optional* | Turso database authentication bearer token. |
| `DATABASE_PATH` | `tgdrive.db` | Local SQLite database file path when Turso is not configured. |
| `CHUNK_SIZE` | `16777216` (16MB) | Raw chunk size in bytes. Set to `8388608` (8MB) or `4194304` (4MB) for faster video buffering. |
| `CACHE_MAX_BYTES` | `67108864` (64MB) | In-memory LRU cache limit for decrypted chunks. Set to `134217728` (128MB) for smoother streaming. |
| `MAX_CONCURRENT_UPLOADS` | `4` | Maximum parallel upload chunks processed by the server at once. |
| `MAX_CONCURRENT_DOWNLOADS` | `4` | Maximum simultaneous download/stream chunks fetched at once. |
| `MAX_MEMORY_BUDGET_BYTES` | `402653184` (384MB) | Memory admission budget to prevent container OOMs on 512MB RAM instances. |
| `PORT` | `8080` | HTTP port the server listens on. |
| `ALLOWED_ORIGIN` | `*` | CORS origin allowed for API access. |

---

## ⚡ Streaming & Performance Tuning

### 1. Optimize Video Startup Time (Time-To-First-Frame)
By default, TgDrive uses 16MB chunks. Because of AES-256-GCM AEAD encryption, the server must download and authenticate the entire chunk before decrypting the first byte.

* **On standard connections (10–20 Mbps):** Set `CHUNK_SIZE=4194304` (4MB) or `8388608` (8MB) in your environment variables.
* **Impact:** Startup latency drops by **75%** (from 11 seconds down to ~2.8 seconds), and seeking becomes instantaneous.

### 2. MP4 FastStart (`moov` Atom Position)
Standard MP4 recordings often place the index of video frames (`moov` atom) at the end of the file. Before uploading large videos, optimize them with `ffmpeg`:
```bash
ffmpeg -i input.mp4 -c copy -movflags +faststart output.mp4
```
This moves the index to byte 0, enabling the video player to start playing without having to request the end of the file first.

### 3. Edge Acceleration with Cloudflare (Free)
If your Render or VPS server is hosted far from your physical location:
1. Route your custom domain through **Cloudflare** (with proxy enabled 🟠).
2. Cloudflare terminates connections locally with **HTTP/3 (QUIC)** and Anycast routing, minimizing mobile network jitter and TCP latency.

---

## 🤖 Bot Pool & Hot Rotation

TgDrive includes a native Bot Pool management interface (via the gear icon in the dashboard):

1. **Add Extra Bots:** Enter new bot tokens in the UI or set `TG_BOT_TOKENS=tok1,tok2,tok3`. The system automatically load-balances uploads across all active bots.
2. **Zero-Downtime Rotation:** If a bot token is compromised or revoked, add the new bot first, then trigger a chunk migration:
   ```http
   POST /api/bots/{old_bot_hash}/migrate
   ```
   TgDrive automatically copies the ciphertext chunks to the new bot in the background without corrupting or deleting original files.

---

## 📡 API Overview

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `POST` | `/api/auth/login` | Authenticate with `ADMIN_PASSWORD` and receive JWT. |
| `GET` | `/api/files` | List files and folders with metadata. |
| `POST` | `/api/files/upload/init` | Initialize a resumable chunked upload session. |
| `PUT` | `/api/files/upload/{id}/chunks/{idx}` | Upload an encrypted chunk block. |
| `GET` | `/api/files/upload/{id}/status` | Check confirmed chunks and resume state. |
| `POST` | `/api/files/upload/{id}/finalize` | Verify and assemble all chunks into a completed file. |
| `POST` | `/api/files/upload/{id}/cancel` | Cancel upload session and delete partial Telegram chunks. |
| `GET` | `/api/files/{id}/stream` | Stream audio/video with HTTP Range support (`206 Partial Content`). |
| `GET` | `/api/files/{id}/thumbnail` | Retrieve decrypted thumbnail image. |
| `DELETE` | `/api/files/{id}` | Delete file from database and purge messages from Telegram. |
| `GET` | `/api/folders` | List directory structure. |
| `POST` | `/api/folders` | Create a new folder. |
| `DELETE` | `/api/folders/{id}` | Recursively delete folder and all enclosed Telegram chunks. |
| `GET` | `/api/bots` | List active bot workers and their health metrics. |
| `POST` | `/api/bots` | Add a new bot token to the active pool. |
| `DELETE` | `/api/bots/{hash}` | Remove a bot from the pool. |
| `POST` | `/api/bots/{hash}/migrate` | Live-migrate chunks from one bot to another. |
| `GET` | `/api/stats` | Retrieve total storage consumption and file counts. |
| `GET` | `/api/system/memory` | Inspect real-time container RSS, Go heap, and cache stats. |

---

## 🔒 Security Policy

For a full breakdown of TgDrive's cryptographic model, threat vectors, and engineering roadmap, consult [docs/ENCRYPTION_ARCHITECTURE.md](docs/ENCRYPTION_ARCHITECTURE.md).

* **Server Trust:** TgDrive provides authenticated storage encryption between your server and Telegram. You must secure your Go backend server and keep `ENCRYPTION_SECRET` and `BOT_CREDENTIAL_SECRET` private.
* **Bug Reports:** To report a security vulnerability, please open a private GitHub security advisory.

---

## 📄 License

This project is licensed under the [MIT License](LICENSE).
Feel free to star ⭐ the repository if you find TgDrive useful!
