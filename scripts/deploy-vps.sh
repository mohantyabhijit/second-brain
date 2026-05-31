#!/usr/bin/env bash
set -euo pipefail

required=(
  DEPLOY_HOST
  DEPLOY_PORT
  DEPLOY_USER
  DEPLOY_SSH_KEY
  ONECLI_API_KEY
  SUPABASE_DB_URL
  SUPABASE_URL
  SUPABASE_SERVICE_ROLE_KEY
)

for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    echo "$name is required" >&2
    exit 1
  fi
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARTIFACT_DIR="$ROOT_DIR/artifacts"
SSH_DIR="$HOME/.ssh"
KEY_PATH="$SSH_DIR/second-brain-deploy"

mkdir -p "$SSH_DIR"
printf '%s\n' "$DEPLOY_SSH_KEY" > "$KEY_PATH"
chmod 600 "$KEY_PATH"
ssh-keyscan -p "$DEPLOY_PORT" "$DEPLOY_HOST" >> "$SSH_DIR/known_hosts"

SSH=(ssh -i "$KEY_PATH" -p "$DEPLOY_PORT" "$DEPLOY_USER@$DEPLOY_HOST")
SCP=(scp -i "$KEY_PATH" -P "$DEPLOY_PORT")
RSYNC=(rsync -az --delete -e "ssh -i $KEY_PATH -p $DEPLOY_PORT")

release="$(date -u +%Y%m%d%H%M%S)"
remote_base="/srv/second-brain"
remote_release="$remote_base/frontend/releases/$release"
remote_tmp="$remote_base/tmp/$release"

"${SSH[@]}" "sudo mkdir -p '$remote_base' && sudo chown -R '$DEPLOY_USER:$DEPLOY_USER' '$remote_base'"
"${SSH[@]}" "mkdir -p '$remote_release' '$remote_tmp' '$remote_base/api' '$remote_base/frontend/releases' '$remote_base/migrations'"

mkdir -p "$ARTIFACT_DIR/frontend"
tar -xzf "$ARTIFACT_DIR/frontend.tgz" -C "$ARTIFACT_DIR/frontend"
"${RSYNC[@]}" "$ARTIFACT_DIR/frontend/" "$DEPLOY_USER@$DEPLOY_HOST:$remote_release/"

"${SCP[@]}" \
  "$ARTIFACT_DIR/api.tgz" \
  "$ARTIFACT_DIR/migrations.tgz" \
  "$ARTIFACT_DIR/runtime/second-brain.env" \
  "$ARTIFACT_DIR/runtime/onecli-api-key" \
  "$DEPLOY_USER@$DEPLOY_HOST:$remote_tmp/"

"${SSH[@]}" "RELEASE='$release' bash -s" <<'REMOTE'
set -euo pipefail

base="/srv/second-brain"
release="${RELEASE:?}"
tmp="$base/tmp/$release"
api_dir="$base/api"
frontend_release="$base/frontend/releases/$release"
onecli="/home/deploy/.local/bin/onecli"

mkdir -p "$api_dir" "$base/migrations"
tar -xzf "$tmp/api.tgz" -C "$api_dir"
rm -rf "$base/migrations.new"
mkdir -p "$base/migrations.new"
tar -xzf "$tmp/migrations.tgz" -C "$base/migrations.new"
rm -rf "$base/migrations"
mv "$base/migrations.new/migrations" "$base/migrations"
rm -rf "$base/migrations.new"
mv "$tmp/second-brain.env" "$api_dir/second-brain.env"
chmod 600 "$api_dir/second-brain.env"
chmod +x \
  "$api_dir/second-brain-api" \
  "$api_dir/second-brain-migrate" \
  "$api_dir/second-brain-refresh" \
  "$api_dir/second-brain-digest" \
  "$api_dir/second-brain-graph-sync" \
  "$api_dir/second-brain-worker" \
  "$api_dir/second-brain-x-token-import"
onecli_api_key="$(cat "$tmp/onecli-api-key")"
rm -f "$tmp/onecli-api-key"

if [[ ! -x "$onecli" ]]; then
  curl -fsSL https://onecli.sh/cli/install | sh
fi
"$onecli" auth login --api-key "$onecli_api_key" >/dev/null
unset onecli_api_key

if ! command -v redis-server >/dev/null 2>&1; then
  sudo apt-get update
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y redis-server
fi
sudo systemctl enable redis-server >/dev/null
sudo systemctl start redis-server
redis-cli ping >/dev/null

set -a
# shellcheck disable=SC1091
. "$api_dir/second-brain.env"
set +a
"$api_dir/second-brain-migrate" "$base/migrations"

cat > /tmp/second-brain-api.service <<SERVICE
[Unit]
Description=Second Brain API
After=network.target redis-server.service
Wants=redis-server.service

[Service]
Type=simple
User=deploy
WorkingDirectory=$api_dir
EnvironmentFile=$api_dir/second-brain.env
ExecStart=$onecli run --project second-brain -- $api_dir/second-brain-api
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
SERVICE
sudo install -m 0644 /tmp/second-brain-api.service /etc/systemd/system/second-brain-api.service
sudo systemctl daemon-reload
sudo systemctl enable second-brain-api >/dev/null
sudo systemctl restart second-brain-api

cat > /tmp/second-brain-worker.service <<SERVICE
[Unit]
Description=Second Brain rcron self-organizing worker
After=network-online.target redis-server.service second-brain-api.service
Wants=network-online.target redis-server.service

[Service]
Type=simple
User=deploy
WorkingDirectory=$api_dir
EnvironmentFile=$api_dir/second-brain.env
ExecStart=$onecli run --project second-brain -- $api_dir/second-brain-worker
Restart=on-failure
RestartSec=30

[Install]
WantedBy=multi-user.target
SERVICE
sudo install -m 0644 /tmp/second-brain-worker.service /etc/systemd/system/second-brain-worker.service
sudo systemctl disable --now second-brain-cycle.timer >/dev/null 2>&1 || true
sudo rm -f /etc/systemd/system/second-brain-cycle.timer /etc/systemd/system/second-brain-cycle.service
sudo rm -f /etc/cron.d/second-brain
sudo systemctl daemon-reload
sudo systemctl enable second-brain-worker >/dev/null
sudo systemctl restart second-brain-worker

ln -sfn "$frontend_release" "$base/frontend/current"
find "$base/frontend/releases" -mindepth 1 -maxdepth 1 -type d | sort -r | tail -n +8 | xargs -r rm -rf

cat > /tmp/second-brain-nginx-block <<'NGINX'
    # BEGIN second-brain
    location = /second-brain {
        return 301 /second-brain/;
    }

    location /second-brain/api/ {
        proxy_pass http://127.0.0.1:8090/api/;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 180s;
        proxy_send_timeout 180s;
    }

    location /second-brain/_next/static/ {
        alias /srv/second-brain/frontend/current/_next/static/;
        add_header Cache-Control "public, max-age=31536000, immutable" always;
        try_files $uri =404;
    }

    location /second-brain/ {
        alias /srv/second-brain/frontend/current/;
        add_header Cache-Control "public, max-age=60, s-maxage=300, stale-while-revalidate=1800" always;
        try_files $uri $uri/ =404;
    }
    # END second-brain

NGINX

nginx_site="/etc/nginx/sites-available/abhijitmohanty.com"
sudo cp "$nginx_site" /tmp/abhijitmohanty.com.new
sudo chown deploy:deploy /tmp/abhijitmohanty.com.new
python3 <<'PY'
from pathlib import Path

site = Path("/tmp/abhijitmohanty.com.new")
block = Path("/tmp/second-brain-nginx-block").read_text()
text = site.read_text()
start = "    # BEGIN second-brain"
end = "    # END second-brain"
while start in text:
    start_index = text.index(start)
    end_index = text.index(end, start_index) + len(end)
    text = text[:start_index].rstrip() + "\n\n" + text[end_index:].lstrip("\n")

anchor = "    # Static files with 404 fallback"
text = text.replace("}" + anchor, "}\n\n" + anchor)
if anchor not in text:
    raise SystemExit(f"nginx insertion anchor not found: {anchor}")

site.write_text(text.replace(anchor, block + anchor, 1))
PY
sudo install -m 0644 /tmp/abhijitmohanty.com.new "$nginx_site"
sudo nginx -t
sudo systemctl reload nginx

rm -rf "$tmp"
REMOTE

echo "Second Brain deployed to https://abhijitmohanty.com/second-brain/"
