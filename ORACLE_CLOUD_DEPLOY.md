# ☁️ Oracle Cloud "Always Free" Deployment Guide for TgDrive

This guide walks you through setting up a **100% free lifelong personal cloud storage server** using Oracle Cloud Always Free Tier.

---

## 1. Create Your Free Oracle Cloud Account
1. Visit [oracle.com/cloud/free](https://www.oracle.com/cloud/free/) and sign up.
2. Select your Home Region:
   - **Recommended**: **Frankfurt (Germany)** or **Amsterdam (Netherlands)** — these provide single-digit millisecond latency to Telegram's European Bot API datacenters.
3. Complete phone verification and add a debit/credit card (used strictly for identity verification, $0 charged).

---

## 2. Launch an "Always Free" Compute Instance
1. In Oracle Cloud Console, navigate to **Compute > Instances > Create Instance**.
2. **Placement**: Keep default Availability Domain.
3. **Image and Shape**:
   - **Image**: `Ubuntu 24.04 LTS` (or `Ubuntu 22.04 LTS`).
   - **Shape**: Choose **Ampere (ARM)**:
     - 2 to 4 OCPUs
     - 12 GB to 24 GB RAM (Always Free allows up to 4 OCPUs and 24 GB RAM free forever).
     *(Alternatively, standard AMD `VM.Standard.E2.1.Micro` with 1 GB RAM is also Always Free).*
4. **Networking**: Ensure **Assign a public IPv4 address** is checked.
5. **Add SSH Keys**: Download and save the private SSH key (`ssh-key.key`) to your computer.
6. Click **Create** and wait 60 seconds for the instance to transition to **RUNNING**. Note your instance's **Public IP**.

---

## 3. Open Ports 80 and 443 in Oracle Firewall

By default, Oracle Cloud blocks inbound web traffic. Open ports 80 and 443:

1. On the Instance details page, click your **Subnet** link.
2. Click your **Default Security List**.
3. Under **Ingress Rules**, click **Add Ingress Rules**:
   - **Source CIDR**: `0.0.0.0/0`
   - **IP Protocol**: `TCP`
   - **Destination Port Range**: `80,443`
   - **Description**: `HTTP and HTTPS for TgDrive`
4. Click **Add Ingress Rules**.

---

## 4. SSH into Your Server & Install Docker

From PowerShell or Terminal on your computer:
```bash
ssh -i /path/to/ssh-key.key ubuntu@<YOUR_PUBLIC_IP>
```

Once logged into your Ubuntu server, run:
```bash
# Update packages and install Docker
sudo apt update && sudo apt upgrade -y
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER

# Allow HTTP/HTTPS in local Ubuntu iptables
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT 6 -m state --state NEW -p tcp --dport 443 -j ACCEPT
sudo netfilter-persistent save
```

Log out and log back in so docker group permissions take effect:
```bash
exit
ssh -i /path/to/ssh-key.key ubuntu@<YOUR_PUBLIC_IP>
```

---

## 5. Clone Repository & Deploy TgDrive

```bash
# Clone the repository
git clone https://github.com/yourusername/TgDrive.git
cd TgDrive

# Configure environment variables
cat << 'EOF' > .env
TG_BOT_TOKEN=your_telegram_bot_token_here
TG_CHANNEL_ID=-1001234567890
ENCRYPTION_SECRET=your_super_secure_random_secret_min_32_chars
BOT_CREDENTIAL_SECRET=your_dedicated_bot_secret_min_32_chars
ADMIN_PASSWORD=your_secure_admin_password
PORT=8080
DATABASE_PATH=/app/data/tgdrive.db
ALLOWED_ORIGIN=*
EOF

# Start TgDrive
docker compose up -d --build
```

---

## 6. Access Your Cloud Storage

Open your browser and navigate to:
```
http://<YOUR_ORACLE_PUBLIC_IP>
```

Log in using your admin password. Your TgDrive is now running 24/7 on dedicated gigabit datacenter fiber with permanent free storage!
