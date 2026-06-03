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

redact_output_value() {
  local value="$1"

  if [ -n "${CF_PAGES_E2E_PROJECT:-}" ]; then
    value="${value//${CF_PAGES_E2E_PROJECT}/***}"
  fi

  echo "${value}"
}

write_success_summary() {
  local url_source="$1"
  local validated_url="$2"

  if [ -z "${GITHUB_STEP_SUMMARY:-}" ]; then
    return 0
  fi

  {
    echo "## Release Smoke"
    echo
    echo "- Result: success"
    echo "- URL source: \`${url_source}\`"
    echo "- Validated URL: $(redact_output_value "${validated_url}")"
    echo "- Namespace: \`${NAMESPACE}\`"
    echo "- Marker group: \`${MARKER_GROUP}\`"
    echo "- Run marker: \`${RUN_MARKER}\`"
    echo "- Image: \`${IMAGE_REPOSITORY}@${IMAGE_DIGEST}\`"
  } >> "${GITHUB_STEP_SUMMARY}"
}

write_failure_summary() {
  local last_log_url="$1"
  local fallback_url="$2"

  if [ -z "${GITHUB_STEP_SUMMARY:-}" ]; then
    return 0
  fi

  {
    echo "## Release Smoke Failed"
    echo
    echo "- Namespace: \`${NAMESPACE}\`"
    echo "- Marker group: \`${MARKER_GROUP}\`"
    echo "- Run marker: \`${RUN_MARKER}\`"
    echo "- Image: \`${IMAGE_REPOSITORY}@${IMAGE_DIGEST}\`"
    if [ -n "${last_log_url}" ]; then
      echo "- Last deployment URL from logs: $(redact_output_value "${last_log_url}")"
    else
      echo "- Last deployment URL from logs: unavailable"
    fi
    echo "- Fallback URL: $(redact_output_value "${fallback_url}")"
  } >> "${GITHUB_STEP_SUMMARY}"
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
    --set config.includeBuiltins=true \
    --set config.includeKustomize=true
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
  if [ "${RELEASE_SMOKE_VERBOSE_CURL:-}" = "true" ]; then
    curl -fsSL --retry 3 --retry-delay 2 "${url}"
  else
    curl -fsSL --retry 3 --retry-delay 2 "${url}" 2>/dev/null
  fi
}

assert_url_contains() {
  local url="$1"
  local expected="$2"
  local body
  body="$(fetch "${url}")" || return 1
  if ! grep -Fq "${expected}" <<<"${body}"; then
    echo "Expected $(redact_output_value "${url}") to contain ${expected}" >&2
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

  assert_url_contains "${base}/index.html" "${MARKER_GROUP}" || return 1
  assert_url_contains "${base}/${MARKER_GROUP}/releasesmoke_v1.json" "${RUN_MARKER}" || return 1
  assert_url_contains "${base}/${MARKER_GROUP}/releasesmoke_v1.html" "${RUN_MARKER}" || return 1
  assert_url_contains "${base}/master-standalone/${MARKER_GROUP}-releasesmoke-stable-v1.json" "${RUN_MARKER}" || return 1
  assert_url_reachable "${base}/core/pod_v1.json" || return 1
  assert_url_reachable "${base}/core/pod_v1.html" || return 1
  assert_url_reachable "${base}/kustomize.config.k8s.io/kustomization_v1beta1.json" || return 1
  assert_url_reachable "${base}/kustomize.config.k8s.io/kustomization_v1beta1.html" || return 1
  assert_url_reachable "${base}/kustomize.config.k8s.io/component_v1alpha1.json" || return 1
  assert_url_reachable "${base}/kustomize.config.k8s.io/component_v1alpha1.html" || return 1
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
  local last_log_url=""
  local log_status=""
  local i

  for i in $(seq 1 48); do
    log_url="$(deployment_url_from_logs)"

    if [ -n "${log_url}" ]; then
      last_log_url="${log_url}"
      if validate_site_at "${log_url}"; then
        echo "Release smoke validated deployment URL: $(redact_output_value "${log_url}")"
        write_success_summary "deployment log" "${log_url}"
        return 0
      fi
      log_status="deployment URL failed"
    else
      log_status="deployment URL unavailable"
    fi

    if validate_site_at "${fixed_url}"; then
      echo "Release smoke validated fallback project URL: $(redact_output_value "${fixed_url}")"
      write_success_summary "fallback project domain" "${fixed_url}"
      return 0
    fi

    echo "Attempt ${i}/48: marker ${RUN_MARKER} not visible yet (${log_status}; fallback failed)"
    sleep 5
  done

  write_failure_summary "${last_log_url}" "${fixed_url}"
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
