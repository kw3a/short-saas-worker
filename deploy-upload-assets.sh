#!/usr/bin/env bash
# Uploads go-video's background videos + background music to the Dokploy
# host, at the paths deploy-docker-compose.yml bind-mounts into the go-video
# container (/data/backgrounds, /data/musics). Run this once before the first
# deploy, and again whenever assets/*.mp4 or assets/*.mp3 change — the compose
# file's bind mounts pick up new files immediately, no redeploy needed.
#
# Usage:
#   ./deploy-upload-assets.sh user@your-server-ip
#   ./deploy-upload-assets.sh user@your-server-ip -p 2222   # extra ssh(1) flags
#
# Source files come from video-server/{backgrounds,musics} (the same assets
# used for local go-video testing — see go-video/README.md, "Develop").

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 user@host [ssh options...]" >&2
  exit 1
fi

target="$1"
shift
ssh_opts=("$@")

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
backgrounds_src="$script_dir/video-server/backgrounds"
musics_src="$script_dir/video-server/musics"

echo "==> Creating /data/backgrounds and /data/musics on $target"
ssh "${ssh_opts[@]}" "$target" \
  'mkdir -p /data/backgrounds /data/musics 2>/dev/null || sudo mkdir -p /data/backgrounds /data/musics'

echo "==> Uploading backgrounds (*.mp4)"
rsync -avz --progress -e "ssh ${ssh_opts[*]}" \
  --include="*.mp4" --exclude="*" \
  "$backgrounds_src/" "$target:/data/backgrounds/"

echo "==> Uploading music (*.mp3)"
rsync -avz --progress -e "ssh ${ssh_opts[*]}" \
  --include="*.mp3" --exclude="*" \
  "$musics_src/" "$target:/data/musics/"

echo "==> Done. Valid names (go-video/internal/httpapi/validate.go):"
echo "    backgrounds: gtav, minecraft, roblox, subways, satisfying"
echo "    musics:      elevator, else, hiddenagenda, nocturne, sneakysnitch, tiptoes, wiener, waltz"
