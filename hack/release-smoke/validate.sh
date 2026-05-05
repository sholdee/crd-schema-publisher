#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
FIXTURE="${ROOT_DIR}/hack/release-smoke/marker-crd.yaml"

required_env=(
  IMAGE_REPOSITORY
  IMAGE_DIGEST
  CF_PAGES_E2E_ACCOUNT_ID
  CF_PAGES_E2E_API_TOKEN
  CF_PAGES_E2E_PROJECT
  RUN_MARKER
  MARKER_GROUP
  NAMESPACE
)

required_tools=(
  curl
  grep
  helm
  kind
  kubectl
  sed
)

fail() {
  echo "release smoke error: $*" >&2
  exit 1
}

require_env() {
  local name
  for name in "${required_env[@]}"; do
    if [ -z "${!name:-}" ]; then
      fail "required environment variable ${name} is not set"
    fi
  done
}

require_tools() {
  local tool
  for tool in "${required_tools[@]}"; do
    if ! command -v "${tool}" >/dev/null 2>&1; then
      fail "required tool ${tool} is not installed"
    fi
  done
}

render_marker_crd() {
  sed \
    -e "s/__MARKER_GROUP__/${MARKER_GROUP}/g" \
    -e "s/__RUN_MARKER__/${RUN_MARKER}/g" \
    "${FIXTURE}"
}

create_cloudflare_secret() {
  kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
  kubectl -n "${NAMESPACE}" create secret generic cloudflare-pages-e2e \
    --from-literal=CLOUDFLARE_ACCOUNT_ID="${CF_PAGES_E2E_ACCOUNT_ID}" \
    --from-literal=CLOUDFLARE_API_TOKEN="${CF_PAGES_E2E_API_TOKEN}" \
    --dry-run=client -o yaml | kubectl apply -f -
}

install_marker_crd() {
  local rendered
  rendered="$(mktemp)"
  render_marker_crd > "${rendered}" || {
    rm -f "${rendered}"
    return 1
  }
  trap 'rm -f "${rendered}"; trap - ERR RETURN' ERR RETURN

  kubectl apply -f "${rendered}"
  kubectl wait \
    --for=condition=Established \
    "crd/releasesmokes.${MARKER_GROUP}" \
    --timeout=60s
}

install_chart() {
  helm upgrade --install crd-schema-publisher "${ROOT_DIR}/charts/crd-schema-publisher" \
    --namespace "${NAMESPACE}" \
    --create-namespace \
    --wait \
    --timeout 2m \
    --set mode=controller \
    --set replicaCount=1 \
    --set image.repository="${IMAGE_REPOSITORY}" \
    --set-string image.digest="${IMAGE_DIGEST}" \
    --set image.pullPolicy=Always \
    --set existingSecret.name=cloudflare-pages-e2e \
    --set config.cfPagesProject="${CF_PAGES_E2E_PROJECT}" \
    --set-string config.debounceSeconds=1 \
    --set-string config.filter.group="${MARKER_GROUP}"
}

deployment_logs() {
  kubectl -n "${NAMESPACE}" logs deployment/crd-schema-publisher --all-containers=true --since=10m 2>/dev/null || true
}

deployment_url_from_logs() {
  deployment_logs \
    | sed -n 's/.*"url":"\([^"]*\)".*/\1/p' \
    | tail -n 1
}

fetch() {
  local url="$1"
  curl -fsSL --retry 3 --retry-delay 2 "${url}"
}

assert_url_contains() {
  local url="$1"
  local expected="$2"
  local body
  body="$(fetch "${url}")" || return 1
  if ! grep -Fq "${expected}" <<<"${body}"; then
    echo "Expected ${url} to contain ${expected}" >&2
    return 1
  fi
}

assert_url_reachable() {
  local url="$1"
  fetch "${url}" >/dev/null
}

validate_site_at() {
  local raw_base="$1"
  local base="${raw_base%/}"

  echo "Validating release smoke site at ${base}"
  assert_url_contains "${base}/index.html" "${MARKER_GROUP}" || return 1
  assert_url_contains "${base}/${MARKER_GROUP}/releasesmoke_v1.json" "${RUN_MARKER}" || return 1
  assert_url_contains "${base}/${MARKER_GROUP}/releasesmoke_v1.html" "${RUN_MARKER}" || return 1
  assert_url_contains "${base}/master-standalone/${MARKER_GROUP}-releasesmoke-stable-v1.json" "${RUN_MARKER}" || return 1
  assert_url_reachable "${base}/schema-search.js" || return 1
  assert_url_reachable "${base}/favicon.svg" || return 1
}

dump_debug_state() {
  echo "::group::release smoke kubernetes state"
  kubectl -n "${NAMESPACE}" get all || true
  kubectl get "crd/releasesmokes.${MARKER_GROUP}" -o yaml || true
  echo "::endgroup::"

  echo "::group::release smoke logs"
  deployment_logs || true
  echo "::endgroup::"
}

wait_for_site() {
  local fixed_url="https://${CF_PAGES_E2E_PROJECT}.pages.dev"
  local log_url=""
  local i

  for i in $(seq 1 48); do
    log_url="$(deployment_url_from_logs)"

    if [ -n "${log_url}" ] && validate_site_at "${log_url}"; then
      echo "Release smoke validated deployment URL: ${log_url}"
      return 0
    fi

    if validate_site_at "${fixed_url}"; then
      echo "Release smoke validated fallback project URL: ${fixed_url}"
      return 0
    fi

    echo "Waiting for current marker ${RUN_MARKER} to be published (${i}/48)"
    sleep 5
  done

  dump_debug_state
  fail "published Cloudflare Pages site did not expose marker ${RUN_MARKER}"
}

main() {
  require_env
  require_tools

  if [ ! -f "${FIXTURE}" ]; then
    fail "marker fixture not found at ${FIXTURE}"
  fi

  create_cloudflare_secret
  install_marker_crd
  install_chart
  wait_for_site
}

main "$@"
