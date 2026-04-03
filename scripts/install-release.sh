#!/usr/bin/env bash
# Install Saddlebag.app and sb from a GitHub Release (works with private repos when authenticated).
#
# Auth (pick one):
#   - gh auth login
#   - export GITHUB_TOKEN=ghp_...   (classic: repo scope; fine-grained: Contents read)
#   - export GH_TOKEN=...
#
# Usage:
#   ./scripts/install-release.sh [v1.0.0]
#   ./scripts/install-release.sh --version v1.0.0 --user
#
set -euo pipefail

REPO="${SADDLEBAG_INSTALL_REPO:-marcusguttenplan/saddlebag}"
VERSION=""
PREFIX="/usr/local/bin"
USER_INSTALL=0
SKIP_APP=0
SKIP_CLI=0
DRY_RUN=0

die() { echo "install-release: $*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Install Saddlebag.app and sb from GitHub Releases (private repo: use gh auth or GITHUB_TOKEN).

Options:
  -v, --version TAG   Release tag (default: latest)
  --prefix DIR        Install sb to DIR (default: /usr/local/bin)
  --user              Install sb to ~/.local/bin (no sudo)
  --skip-app          Only install sb
  --skip-cli          Only install Saddlebag.app
  --dry-run           Print actions only
  -h, --help          This help

Examples:
  gh auth login
  ./scripts/install-release.sh v1.0.0

  GITHUB_TOKEN=ghp_xxx ./scripts/install-release.sh --user
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)
      [[ $# -ge 2 ]] || die "--version needs a value"
      VERSION=$2
      shift 2
      ;;
    --prefix)
      [[ $# -ge 2 ]] || die "--prefix needs a value"
      PREFIX=$2
      shift 2
      ;;
    --user)
      USER_INSTALL=1
      shift
      ;;
    --skip-app)
      SKIP_APP=1
      shift
      ;;
    --skip-cli)
      SKIP_CLI=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      [[ -z "$VERSION" ]] || die "unexpected argument: $1 (version already set to $VERSION)"
      VERSION=$1
      shift
      ;;
  esac
done

auth_token() {
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    printf '%s' "$GITHUB_TOKEN"
    return
  fi
  if [[ -n "${GH_TOKEN:-}" ]]; then
    printf '%s' "$GH_TOKEN"
    return
  fi
  if command -v gh >/dev/null 2>&1; then
    gh auth token 2>/dev/null && return
  fi
  return 1
}

TOKEN="$(auth_token || true)"
[[ -n "$TOKEN" ]] || die "private repo: run \`gh auth login\` or set GITHUB_TOKEN (see script header)"

resolve_version() {
  if [[ -n "$VERSION" ]]; then
    [[ "$VERSION" == v* ]] || VERSION="v${VERSION}"
    printf '%s' "$VERSION"
    return
  fi
  curl -fsSL \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Accept: application/vnd.github+json" \
    -H "X-GitHub-Api-Version: 2022-11-28" \
    "https://api.github.com/repos/${REPO}/releases/latest" |
    python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])'
}

machine_arch() {
  case "$(uname -m)" in
    arm64) printf '%s' arm64 ;;
    x86_64) printf '%s' amd64 ;;
    *) die "unsupported arch: $(uname -m)" ;;
  esac
}

download() {
  local url=$1 dest=$2
  local asset_name="$(basename "$url")"
  
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] download $asset_name -> $dest"
    return
  fi

  # 1. Try gh CLI if available and authenticated
  if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
    if gh release download "$TAG" -p "$asset_name" --repo "$REPO" --clobber -O "$dest" >/dev/null 2>&1; then
      return 0
    fi
  fi

  # 2. Try GitHub API to avoid 404 on private repositories
  if [[ -n "${TOKEN:-}" ]]; then
    local release_url="https://api.github.com/repos/${REPO}/releases/tags/${TAG}"
    # Extract asset ID
    local asset_id=$(curl -fsSL -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/vnd.github+json" "$release_url" | python3 -c "import json,sys; data=json.load(sys.stdin); print(next((a['id'] for a in data.get('assets',[]) if a['name'] == '${asset_name}'), ''))" 2>/dev/null || true)
    
    if [[ -n "$asset_id" ]]; then
      local api_url="https://api.github.com/repos/${REPO}/releases/assets/${asset_id}"
      curl -fsSL -H "Authorization: Bearer ${TOKEN}" -H "Accept: application/octet-stream" -o "$dest" "$api_url"
      return $?
    fi
  fi

  # 3. Fallback to direct url (often fails on private repos)
  curl -fsSL \
    ${TOKEN:+-H "Authorization: Bearer ${TOKEN}"} \
    -H "Accept: application/octet-stream" \
    -o "$dest" \
    "$url"
}

TAG="$(resolve_version)"
BASE="https://github.com/${REPO}/releases/download/${TAG}"
SB_ARCH="$(machine_arch)"
SB_ZIP="sb-darwin-${SB_ARCH}.zip"
SB_BIN="sb-darwin-${SB_ARCH}"

if [[ "$USER_INSTALL" -eq 1 ]]; then
  PREFIX="${HOME}/.local/bin"
fi
mkdir -p "$PREFIX"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/saddlebag-install.XXXXXX")"
cleanup() { rm -rf "$WORK_DIR"; }
trap cleanup EXIT
cd "$WORK_DIR"

if [[ "$SKIP_CLI" -eq 0 ]]; then
  echo "Downloading ${TAG} (${SB_ZIP})..."
  download "${BASE}/${SB_ZIP}" "${SB_ZIP}"
  download "${BASE}/checksums-sha256.txt" checksums-sha256.txt
  if [[ "$DRY_RUN" -ne 1 ]]; then
    grep -F "${SB_ZIP}" checksums-sha256.txt | shasum -a 256 -c ||
      die "checksum mismatch for ${SB_ZIP}"
    unzip -q -o "${SB_ZIP}"
    [[ -f "$SB_BIN" ]] || die "expected ${SB_BIN} inside ${SB_ZIP}"
    chmod +x "$SB_BIN"
    DEST="${PREFIX}/sb"
    if [[ -w "$PREFIX" ]]; then
      install -m 0755 "$SB_BIN" "$DEST"
    else
      sudo install -m 0755 "$SB_BIN" "$DEST"
    fi
    echo "Installed sb -> ${DEST}"
  fi
fi

if [[ "$SKIP_APP" -eq 0 ]]; then
  echo "Downloading ${TAG} (Saddlebag.zip)..."
  download "${BASE}/Saddlebag.zip" Saddlebag.zip
  if [[ "$DRY_RUN" -ne 1 ]]; then
    unzip -q -o Saddlebag.zip
    APP_SRC="Saddlebag.app"
    [[ -d "$APP_SRC" ]] || APP_SRC="$(find . -name Saddlebag.app -type d -maxdepth 4 | head -1)"
    [[ -n "$APP_SRC" && -d "$APP_SRC" ]] || die "Saddlebag.app not found inside Saddlebag.zip"
    APP_DEST="/Applications/Saddlebag.app"
    if [[ -w /Applications ]]; then
      rm -rf "$APP_DEST"
      ditto "$APP_SRC" "$APP_DEST"
    else
      sudo rm -rf "$APP_DEST"
      sudo ditto "$APP_SRC" "$APP_DEST"
    fi
    echo "Installed Saddlebag.app -> ${APP_DEST}"
  fi
fi

echo "Done (${TAG}). Add to your shell rc: eval \"\$(sb init zsh)\"  (or bash)"
