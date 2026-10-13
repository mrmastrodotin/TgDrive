# 🚀 Deploying TgDrive on Koyeb (Free Tier)

Koyeb offers a **free tier** hosted in **Frankfurt, Germany** (sub-5ms latency to Telegram's EU datacenters) — no credit card required when signing in with GitHub.

---

## Prerequisites

- GitHub account with TgDrive pushed to a repository
- Telegram Bot Token & Channel ID (see `.env.example`)
- A strong encryption secret and admin password

---

## Step 1: Push Code to GitHub

```powershell
git add .
git commit -m "feat: production-ready for Koyeb deployment"
git push origin main
```

---

## Step 2: Create Koyeb Account

1. Go to **[app.koyeb.com](https://app.koyeb.com/)**
2. Click **Sign in with GitHub**
3. Authorize Koyeb to access your repositories

---

## Step 3: Deploy TgDrive

1. Click **Create Service** → **GitHub** → select your **TgDrive** repo
2. Build settings:
   - **Builder**: `Dockerfile`
   - **Dockerfile location**: `/Dockerfile` (default)
3. **Region**: `Frankfurt (fra)` — closest to Telegram datacenters
4. **Instance**: `Free / Nano` ($0/month)
5. **Exposed Ports**: `8080` / `HTTP`
6. **Health Check**: Path `/health`, Protocol `HTTP`
7. **Environment Variables**:

| Key | Value | Notes |
| :--- | :--- | :--- |
| `TG_BOT_TOKEN` | `your_bot_token` | From @BotFather |
| `TG_CHANNEL_ID` | `-100xxxxxxxxxx` | Your private channel |
| `ENCRYPTION_SECRET` | `your_secret_key` | Min 32 chars, master key for chunk encryption |
| `BOT_CREDENTIAL_SECRET` | `your_bot_secret` | Min 32 chars, required in production for stored bot credentials |
| `JWT_SECRET` | `your_jwt_secret` | Min 32 chars, dedicated secret for signing JWT tokens |
| `ADMIN_PASSWORD` | `your_password` | Web dashboard login |
| `TURSO_DATABASE_URL` | `libsql://your-db.turso.io` | Required in production for persistent metadata durability |
| `TURSO_AUTH_TOKEN` | `your_turso_token` | Auth token for Turso cloud database |
| `PORT` | `8080` | Must match exposed port |
| `ALLOWED_ORIGIN` | `*` | Or your exact domain |
| `DATABASE_PATH` | `/app/data/tgdrive.db` | Local fallback path (requires ALLOW_LOCAL_SQLITE=true in production) |

8. Click **Deploy**

---

## Step 4: Access Your Live Drive

- Build takes ~2-3 minutes
- Status turns green → **Healthy**
- Public URL: `https://tgdrive-<your-app>.koyeb.app`

---

## ⚠️ Important: Ephemeral Disk on Koyeb Free Tier

Koyeb's free tier uses **ephemeral storage** — the SQLite database resets on every redeploy or container restart. Your actual files are safe in Telegram, but the **metadata** (file index, folder structure) is lost.

### Options for Persistent Metadata

1. **Turso (Recommended, Free)**: Create a free database at [turso.tech](https://turso.tech), then set:
   - `TURSO_DATABASE_URL` = `libsql://your-db-name.turso.io`
   - `TURSO_AUTH_TOKEN` = `your_turso_token`

2. **Koyeb Persistent Volume** (Paid plans only): Attach a volume at `/app/data`

3. **Accept ephemeral** for testing/demo — files remain in Telegram and can be re-indexed
