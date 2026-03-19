# Deploy Etiquetta on Hetzner Cloud

One-command deployment with automatic HTTPS.

## Prerequisites

- A Hetzner Cloud account
- A domain name with DNS access

## Steps

### 1. Point DNS

Create an **A record** for your domain pointing to the server IP you'll get in step 3.
If you don't know the IP yet, create the server first, then update DNS.

### 2. Edit cloud-init.yaml

Open `cloud-init.yaml` and replace `YOUR_DOMAIN` with your actual domain:

```yaml
  - path: /etc/etiquetta/domain
    content: analytics.example.com   # ← your domain here
```

### 3. Create the server

**Via Hetzner Console:**

1. Go to [Hetzner Cloud Console](https://console.hetzner.cloud)
2. Create a new server
3. Choose **Ubuntu 22.04** or **Debian 12**
4. Select a plan (CX22 / 2 vCPU + 4 GB RAM is recommended)
5. Under **Cloud config**, paste the contents of `cloud-init.yaml`
6. Create the server

**Via hcloud CLI:**

```bash
hcloud server create \
  --name etiquetta \
  --type cx22 \
  --image ubuntu-22.04 \
  --location fsn1 \
  --user-data-from-file cloud-init.yaml
```

### 4. Wait ~2 minutes

The server will:
- Install and configure UFW (firewall)
- Install Caddy (reverse proxy with automatic HTTPS)
- Install Etiquetta
- Start both services

### 5. Open your domain

Visit `https://your-domain.com` and complete the setup wizard (create admin account).

## What's included

| Component | Purpose |
|-----------|---------|
| Etiquetta | Analytics server (port 3456, localhost only) |
| Caddy | Reverse proxy, automatic HTTPS via Let's Encrypt |
| UFW | Firewall — only ports 22, 80, 443 open |
| systemd | Auto-restart on crash, start on boot |

## Useful commands

```bash
# View Etiquetta logs
sudo journalctl -u etiquetta -f

# View Caddy logs
sudo journalctl -u caddy -f

# Restart Etiquetta
sudo systemctl restart etiquetta

# Update Etiquetta to latest version
curl -sSL https://raw.githubusercontent.com/caioricciuti/etiquetta/main/install.sh | sudo bash
sudo systemctl restart etiquetta

# Check status
sudo systemctl status etiquetta
sudo systemctl status caddy
```

## Server sizing

| Plan | vCPU | RAM | Good for |
|------|------|-----|----------|
| CX22 | 2 | 4 GB | Up to ~100k pageviews/month |
| CX32 | 4 | 8 GB | Up to ~1M pageviews/month |
| CX42 | 8 | 16 GB | 1M+ pageviews/month |

DuckDB is column-oriented and compresses well, so disk usage stays small even at scale.
