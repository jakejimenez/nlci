#!/bin/sh
set -e

REPO="jakejimenez/nlci"
BIN_DIR="${HOME}/.local/bin"
APPLE_BIN_DIR="${HOME}/.config/nlci/bin"
BINARY="nlci"
APPLE_BINARY="nlci-apple"

# ── colours ────────────────────────────────────────────────────────────────────
bold=$(printf '\033[1m')
green=$(printf '\033[32m')
yellow=$(printf '\033[33m')
red=$(printf '\033[31m')
reset=$(printf '\033[0m')

info()    { printf "${bold}==> %s${reset}\n" "$*"; }
success() { printf "${green}✓ %s${reset}\n" "$*"; }
warn()    { printf "${yellow}! %s${reset}\n" "$*"; }
die()     { printf "${red}error: %s${reset}\n" "$*" >&2; exit 1; }

# ── platform detection ─────────────────────────────────────────────────────────
OS="$(uname -s)"
ARCH="$(uname -m)"

[ "$OS" = "Darwin" ] || die "nlci currently only supports macOS"

case "$ARCH" in
  arm64)  GO_ARCH="arm64" ;;
  x86_64) GO_ARCH="amd64" ;;
  *)      die "Unsupported architecture: $ARCH" ;;
esac

MACOS_VERSION="$(sw_vers -productVersion 2>/dev/null || echo "0")"
MACOS_MAJOR="$(echo "$MACOS_VERSION" | cut -d. -f1)"

CAN_BUILD_APPLE=0
if [ "$ARCH" = "arm64" ] && [ "$MACOS_MAJOR" -ge 26 ] 2>/dev/null; then
  if command -v swift >/dev/null 2>&1; then
    CAN_BUILD_APPLE=1
  fi
fi

# ── helpers ────────────────────────────────────────────────────────────────────
need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1 — please install it and retry"
}

ensure_dir() {
  mkdir -p "$1"
}

# ── step 1: try binary download from GitHub releases ──────────────────────────
try_download() {
  info "Checking for a pre-built release..."

  LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')

  if [ -z "$LATEST" ]; then
    warn "No release found — will build from source"
    return 1
  fi

  TARBALL="nlci-${LATEST}-darwin-${GO_ARCH}.tar.gz"
  URL="https://github.com/${REPO}/releases/download/${LATEST}/${TARBALL}"

  # Check the asset actually exists before downloading
  HTTP_STATUS=$(curl -fsSL -o /dev/null -w "%{http_code}" "$URL" 2>/dev/null || echo "000")
  if [ "$HTTP_STATUS" != "200" ]; then
    warn "Release ${LATEST} has no pre-built binary for darwin/${GO_ARCH} — will build from source"
    return 1
  fi

  info "Downloading ${LATEST} (darwin/${GO_ARCH})..."
  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  curl -fsSL "$URL" -o "${TMP_DIR}/${TARBALL}"
  tar -xzf "${TMP_DIR}/${TARBALL}" -C "$TMP_DIR"

  ensure_dir "$BIN_DIR"
  cp "${TMP_DIR}/${BINARY}" "${BIN_DIR}/${BINARY}"
  chmod 755 "${BIN_DIR}/${BINARY}"
  success "Installed ${BINARY} ${LATEST} → ${BIN_DIR}/${BINARY}"

  if [ -f "${TMP_DIR}/${APPLE_BINARY}" ]; then
    ensure_dir "$APPLE_BIN_DIR"
    cp "${TMP_DIR}/${APPLE_BINARY}" "${APPLE_BIN_DIR}/${APPLE_BINARY}"
    chmod 755 "${APPLE_BIN_DIR}/${APPLE_BINARY}"
    success "Installed ${APPLE_BINARY} → ${APPLE_BIN_DIR}/${APPLE_BINARY}"
  fi

  return 0
}

# ── step 2: build from source ─────────────────────────────────────────────────
build_from_source() {
  info "Building from source..."
  need_cmd go
  need_cmd git

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  info "Cloning ${REPO}..."
  git clone --depth 1 "https://github.com/${REPO}.git" "$TMP_DIR" >/dev/null 2>&1

  info "Building ${BINARY}..."
  (cd "$TMP_DIR" && go build -o "${BINARY}" ./cmd/nlci)

  ensure_dir "$BIN_DIR"
  cp "${TMP_DIR}/${BINARY}" "${BIN_DIR}/${BINARY}"
  chmod 755 "${BIN_DIR}/${BINARY}"
  success "Installed ${BINARY} → ${BIN_DIR}/${BINARY}"

  if [ "$CAN_BUILD_APPLE" = "1" ]; then
    info "Building Apple Intelligence bridge (macOS ${MACOS_VERSION} / Apple Silicon)..."
    if (cd "${TMP_DIR}/apple" && swift build -c release >/dev/null 2>&1); then
      ensure_dir "$APPLE_BIN_DIR"
      cp "${TMP_DIR}/apple/.build/release/${APPLE_BINARY}" "${APPLE_BIN_DIR}/${APPLE_BINARY}"
      chmod 755 "${APPLE_BIN_DIR}/${APPLE_BINARY}"
      success "Installed ${APPLE_BINARY} → ${APPLE_BIN_DIR}/${APPLE_BINARY}"
    else
      warn "Apple bridge build failed — Ollama/llama.cpp/LM Studio backends will still work"
    fi
  else
    if [ "$ARCH" != "arm64" ] || [ "$MACOS_MAJOR" -lt 26 ] 2>/dev/null; then
      warn "Skipping Apple bridge (requires macOS 26 + Apple Silicon)"
    else
      warn "Skipping Apple bridge (swift not found — install Xcode to enable it)"
    fi
  fi
}

# ── step 3: PATH check ─────────────────────────────────────────────────────────
check_path() {
  case ":${PATH}:" in
    *":${BIN_DIR}:"*) ;;
    *)
      printf "\n"
      warn "${BIN_DIR} is not in your PATH."
      warn "Add this to your shell profile (~/.zshrc or ~/.bashrc):"
      printf "\n  ${bold}export PATH=\"\$HOME/.local/bin:\$PATH\"${reset}\n\n"
      warn "Then restart your shell or run: source ~/.zshrc"
      ;;
  esac
}

# ── step 4: verify ─────────────────────────────────────────────────────────────
verify() {
  printf "\n"
  info "Verifying install..."
  if "${BIN_DIR}/${BINARY}" config 2>&1; then
    printf "\n"
    success "nlci is ready. Try: nlci docker \"show me running containers\" --dry-run"
  fi
}

# ── main ───────────────────────────────────────────────────────────────────────
printf "\n${bold}Installing nlci${reset}\n\n"

if ! try_download; then
  build_from_source
fi

check_path
verify
