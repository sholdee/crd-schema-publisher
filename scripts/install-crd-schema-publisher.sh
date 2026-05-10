#!/usr/bin/env bash
set -Eeuo pipefail

REPO="sholdee/crd-schema-publisher"
PROJECT="crd-schema-publisher"
DEFAULT_TARGET="/usr/local/bin/crd-schema-publisher"

ASSET=""
AUTO_YES=false
COSIGN_STATUS="not checked"
DRY_RUN=false
IF_OUTDATED=false
REQUIRE_COSIGN=false
TARGET=""
VERSION=""
WORKDIR=""

SUDO=()

usage() {
  cat <<'EOF'
Install or update the crd-schema-publisher standalone binary.

Usage:
  install-crd-schema-publisher.sh [options]

Options:
  --version <tag>      Install a specific release tag instead of latest
  --target <path>      Install to a specific binary path
  --dry-run            Download and verify only; do not install
  --if-outdated        Exit without changes when the installed binary matches the selected release
  --require-cosign     Fail if cosign verification cannot be completed
  -y, --yes            Accept prompts and run non-interactively
  -h, --help           Show this help

Examples:
  ./install-crd-schema-publisher.sh
  ./install-crd-schema-publisher.sh --yes
  ./install-crd-schema-publisher.sh --version v2026.509.80328
  ./install-crd-schema-publisher.sh --target "$HOME/bin/crd-schema-publisher"
EOF
}

log() { printf " %s\n" "$*"; }
ok() { printf " %s\n" "$*"; }
warn() { printf "  %s\n" "$*" >&2; }
err() { printf " %s\n" "$*" >&2; }

cleanup() {
  if [[ -n "${WORKDIR}" && -d "${WORKDIR}" ]]; then
    rm -rf "${WORKDIR}"
  fi
}

trap cleanup EXIT

set_sudo() {
  if [[ "${EUID}" -eq 0 ]]; then
    SUDO=()
  else
    SUDO=(sudo)
  fi
}

require_sudo() {
  if [[ "${EUID}" -eq 0 ]]; then
    return 0
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    err "This operation requires root privileges, but sudo is not available."
    exit 1
  fi
  sudo -v
}

prompt_yes_no() {
  local prompt="$1"
  local answer

  if [[ "${AUTO_YES}" == "true" ]]; then
    return 0
  fi

  if ! { : </dev/tty >/dev/tty; } 2>/dev/null; then
    err "Refusing to modify the filesystem without an interactive terminal or --yes."
    err "For automation, use: curl -fsSL https://crdsp.shold.io | bash -s -- --yes"
    exit 1
  fi

  printf "%s [Y/n] " "${prompt}" >/dev/tty
  IFS= read -r answer </dev/tty || exit 1
  [[ -z "${answer}" || "${answer}" =~ ^[Yy]$ ]]
}

detect_target_asset() {
  local os raw_arch arch

  case "$(uname -s)" in
    Linux) os="linux" ;;
    Darwin) os="darwin" ;;
    *)
      err "Unsupported OS: $(uname -s). Supported OSes: Linux, Darwin."
      exit 1
      ;;
  esac

  raw_arch="$(uname -m)"
  case "${raw_arch}" in
    amd64|x86_64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *)
      err "Unsupported architecture: ${raw_arch}. Supported architectures: amd64, arm64."
      exit 1
      ;;
  esac

  ASSET="${PROJECT}-${os}-${arch}"
  log "Detected target ${os}/${arch}"
}

resolve_target_path() {
  if [[ -n "${TARGET}" ]]; then
    TARGET_PATH="${TARGET}"
  elif command -v "${PROJECT}" >/dev/null 2>&1; then
    TARGET_PATH="$(command -v "${PROJECT}")"
  else
    TARGET_PATH="${DEFAULT_TARGET}"
  fi

  if [[ "${TARGET_PATH}" != /* ]]; then
    err "Target path must be absolute: ${TARGET_PATH}"
    exit 1
  fi
}

release_base_url() {
  if [[ -n "${VERSION}" ]]; then
    printf "https://github.com/%s/releases/download/%s" "${REPO}" "${VERSION}"
  else
    printf "https://github.com/%s/releases/latest/download" "${REPO}"
  fi
}

release_label() {
  if [[ -n "${VERSION}" ]]; then
    printf "%s" "${VERSION}"
  else
    printf "latest"
  fi
}

ensure_workdir() {
  if [[ -z "${WORKDIR}" ]]; then
    WORKDIR="$(mktemp -d)"
  fi
}

download_checksums() {
  local base_url

  ensure_workdir
  if [[ -f "${WORKDIR}/checksums-sha256.txt" ]]; then
    return 0
  fi

  base_url="$(release_base_url)"
  log "Downloading checksum manifest for ${REPO} $(release_label)"
  curl -fsSL "${base_url}/checksums-sha256.txt" -o "${WORKDIR}/checksums-sha256.txt"
}

download_assets() {
  local base_url

  ensure_workdir
  base_url="$(release_base_url)"

  log "Downloading ${REPO} ${ASSET} from $(release_label)"
  curl -fsSL "${base_url}/${ASSET}" -o "${WORKDIR}/${ASSET}"
  download_checksums
  if curl -fsSL "${base_url}/checksums-sha256.txt.sigstore.json" -o "${WORKDIR}/checksums-sha256.txt.sigstore.json"; then
    :
  else
    rm -f "${WORKDIR}/checksums-sha256.txt.sigstore.json"
  fi
  chmod 0755 "${WORKDIR}/${ASSET}"
  ok "Downloaded release assets"
}

hash_file() {
  local path="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${path}" | awk '{print $1}'
  else
    shasum -a 256 "${path}" | awk '{print $1}'
  fi
}

expected_asset_checksum() {
  local checksum

  checksum="$(awk -v asset="${ASSET}" '$2 == asset {print $1}' "${WORKDIR}/checksums-sha256.txt")"
  if [[ -z "${checksum}" ]]; then
    err "No checksum entry found for ${ASSET}."
    exit 1
  fi

  printf "%s" "${checksum}"
}

target_matches_release() {
  local actual expected

  if [[ ! -f "${TARGET_PATH}" ]]; then
    return 1
  fi

  expected="$(expected_asset_checksum)"
  actual="$(hash_file "${TARGET_PATH}")"
  [[ "${actual}" == "${expected}" ]]
}

maybe_exit_if_current() {
  if [[ "${IF_OUTDATED}" != "true" ]]; then
    return 0
  fi

  download_checksums
  if target_matches_release; then
    ok "${TARGET_PATH} already matches $(release_label); no update needed."
    exit 0
  fi

  if [[ -e "${TARGET_PATH}" ]]; then
    log "${TARGET_PATH} does not match $(release_label); update needed."
  else
    log "${TARGET_PATH} is missing; install needed."
  fi
}

verify_checksum() {
  local actual expected

  expected="$(expected_asset_checksum)"
  actual="$(hash_file "${WORKDIR}/${ASSET}")"
  if [[ "${actual}" != "${expected}" ]]; then
    err "Checksum verification failed for ${ASSET}."
    err "Expected: ${expected}"
    err "Actual:   ${actual}"
    exit 1
  fi
}

verify_cosign() {
  if [[ ! -f "${WORKDIR}/checksums-sha256.txt.sigstore.json" ]]; then
    if [[ "${REQUIRE_COSIGN}" == "true" ]]; then
      err "cosign verification is required but the release has no checksum Sigstore bundle."
      exit 1
    fi
    warn "release has no checksum Sigstore bundle; skipped cosign verification"
    COSIGN_STATUS="skipped; bundle unavailable"
    return 0
  fi

  if ! command -v cosign >/dev/null 2>&1; then
    if [[ "${REQUIRE_COSIGN}" == "true" ]]; then
      err "cosign verification is required but cosign is not installed."
      exit 1
    fi
    warn "cosign not found; skipped checksum bundle verification"
    COSIGN_STATUS="skipped; cosign not found"
    return 0
  fi

  log "Verifying checksum Sigstore bundle"
  cosign verify-blob \
    --bundle "${WORKDIR}/checksums-sha256.txt.sigstore.json" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    --certificate-identity "https://github.com/${REPO}/.github/workflows/release.yaml@refs/heads/main" \
    "${WORKDIR}/checksums-sha256.txt" >/dev/null
  ok "Cosign bundle verified"
  COSIGN_STATUS="verified"
}

verify_assets() {
  log "Verifying checksum"
  verify_checksum
  ok "Checksum verified"

  verify_cosign

  log "Checking downloaded binary"
  "${WORKDIR}/${ASSET}" --version
  ok "Downloaded binary runs"
}

target_metadata() {
  if [[ -e "${TARGET_PATH}" ]]; then
    if stat -c '%u %g %a' "${TARGET_PATH}" >/dev/null 2>&1; then
      stat -c '%u %g %a' "${TARGET_PATH}"
    else
      stat -f '%u %g %Lp' "${TARGET_PATH}"
    fi
  else
    printf "0 0 0755\n"
  fi
}

target_needs_sudo() {
  local dir

  if [[ -e "${TARGET_PATH}" ]]; then
    [[ ! -w "${TARGET_PATH}" ]]
    return
  fi

  dir="$(dirname "${TARGET_PATH}")"
  [[ ! -w "${dir}" ]]
}

install_binary() {
  local group mode owner

  read -r owner group mode < <(target_metadata)

  log "Installing binary to ${TARGET_PATH}"
  mkdir -p "$(dirname "${TARGET_PATH}")" 2>/dev/null || true
  if target_needs_sudo; then
    require_sudo
    "${SUDO[@]}" mkdir -p "$(dirname "${TARGET_PATH}")"
    "${SUDO[@]}" install -o "${owner}" -g "${group}" -m "${mode}" "${WORKDIR}/${ASSET}" "${TARGET_PATH}"
  else
    install -m "${mode}" "${WORKDIR}/${ASSET}" "${TARGET_PATH}"
  fi
  ok "Installed ${TARGET_PATH}"

  log "Installed version"
  "${TARGET_PATH}" --version
}

print_plan() {
  printf "\n"
  printf " Ready to install/update crd-schema-publisher\n"
  printf "\n"
  printf " Release:       %s\n" "$(release_label)"
  printf " Target:        %s\n" "${TARGET_PATH}"
  printf " Binary asset:  %s\n" "${ASSET}"
  printf " Checksum:      verified\n"
  printf " Cosign:        %s\n" "${COSIGN_STATUS}"
  printf "\n"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        VERSION="${2:-}"
        [[ -n "${VERSION}" ]] || { err "--version requires a tag"; exit 1; }
        shift 2
        ;;
      --target)
        TARGET="${2:-}"
        [[ -n "${TARGET}" ]] || { err "--target requires a path"; exit 1; }
        shift 2
        ;;
      --dry-run)
        DRY_RUN=true
        shift
        ;;
      --if-outdated)
        IF_OUTDATED=true
        shift
        ;;
      --require-cosign)
        REQUIRE_COSIGN=true
        shift
        ;;
      -y|--yes)
        AUTO_YES=true
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        err "Unknown argument: $1"
        usage
        exit 1
        ;;
    esac
  done
}

main() {
  parse_args "$@"
  set_sudo

  printf " crd-schema-publisher Installer\n"
  printf "===============================\n\n"

  detect_target_asset
  resolve_target_path
  download_checksums
  maybe_exit_if_current
  download_assets
  verify_assets

  if [[ "${DRY_RUN}" == "true" ]]; then
    ok "Dry run complete. No files changed."
    return 0
  fi

  print_plan
  if ! prompt_yes_no "Continue with install/update?"; then
    err "Aborted."
    exit 1
  fi

  install_binary
  ok "Completed successfully."
}

main "$@"
