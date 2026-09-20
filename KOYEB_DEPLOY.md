# 🚀 Deploying TgDrive on Koyeb (100% Free, Zero Credit Card)

Koyeb offers a **free tier** hosted in **Frankfurt, Germany** (sub-5ms network latency to Telegram's European datacenters) and requires **no credit card** when signing in with GitHub.

---

## Step 1: Push Your Code to GitHub

Open PowerShell in your project folder (`w:\work space\projects\TgDrive`):

```powershell
# 1. Initialize git if not already done
git init
git branch -M main

# 2. Stage and commit files (.env, .db, and keys are automatically ignored)
git add .
git commit -m "feat: complete TgDrive high-speed encrypted cloud storage"

# 3. Create a private repository on github.com/new named 'TgDrive'
# Then link and push:
git remote add origin https://github.com/<YOUR_GITHUB_USERNAME>/TgDrive.git
git push -u origin main
```

---

## Step 2: Create a Free Account on Koyeb

1. Go to **[app.koyeb.com](https://app.koyeb.com/)**.
2. Click **Sign in with GitHub**.
3. Authorize Koyeb to access your repositories (no billing or card requested).

---

## Step 3: Deploy TgDrive Service

1. On the Koyeb dashboard, click **Create Service**.
2. Select **GitHub** as the deployment source and pick your **TgDrive** repository.
3. Configure the build settings:
   - **Builder**: Select **Dockerfile**
   - **Dockerfile location**: Leave as default (`/Dockerfile`)
4. **Regions**: Choose **Frankfurt (fra)** *(optimal speed for Telegram)*.
5. **Instance Type**: Select the **Free / Nano** tier ($0/month).
6. Under **Exposed Ports**:
   - Port: `8080`
   - Protocol: `HTTP` (Koyeb handles edge HTTPS / TLS automatically)
7. Under **Environment Variables**, add the following key-value pairs:

| Key | Value | Description |
| :--- | :--- | :--- |
| `TG_BOT_TOKEN` | `your_bot_token` | Your Telegram Bot Token |
| `TG_CHANNEL_ID` | `-100xxxxxxxxxx` | Your Telegram Channel ID |
| `ENCRYPTION_SECRET` | `your_secret_key` | Master Encryption Key |
| `ADMIN_PASSWORD` | `$@r@|\` | Web Dashboard Password |
| `PORT` | `8080` | Internal Listening Port |
| `ALLOWED_ORIGIN` | `*` | CORS Allowed Origins |
| `DATABASE_PATH` | `tgdrive.db` | Metadata Database |

*(Optional: For permanent metadata persistence across rebuilds without local disk resets, you can provide `TURSO_DATABASE_URL` and `TURSO_AUTH_TOKEN` from free Turso.tech, or stick with `tgdrive.db`).*

8. Click **Deploy**.

---

## Step 4: Access Your Live Personal Drive

* Koyeb will automatically build the React frontend and Go backend container.
* In ~2 minutes, your deployment status will turn green (**Healthy**).
* You will receive a public HTTPS link:
  ```
  https://tgdrive-<your-app>.koyeb.app
  ```
* Open that URL on your phone or computer, log in with `$@r@|\`, and start uploading and streaming files through Telegram at datacenter speeds!
