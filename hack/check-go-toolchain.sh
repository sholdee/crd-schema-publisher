#!/usr/bin/env bash
set -euo pipefail

mod_file="${1:-go.mod}"
dockerfile="${2:-Dockerfile}"

fail() {
  echo "go toolchain check failed: $*" >&2
  exit 1
}

[[ -f "${mod_file}" ]] || fail "missing ${mod_file}"
[[ -f "${dockerfile}" ]] || fail "missing ${dockerfile}"

go_directive="$(
  awk '
    $1 == "go" {
      print $2
      found = 1
      exit
    }
    END {
      if (!found) {
        exit 1
      }
    }
  ' "${mod_file}"
)" || fail "missing go directive in ${mod_file}"

docker_go="$(
  awk '
    $1 == "FROM" {
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^golang:[^@[:space:]]+/) {
          sub(/^golang:/, "", $i)
          sub(/@.*/, "", $i)
          print $i
          exit
        }
      }
    }
  ' "${dockerfile}"
)"

[[ -n "${docker_go}" ]] || fail "missing golang build image in ${dockerfile}"
[[ "${docker_go}" == "${go_directive}" ]] ||
  fail "Dockerfile golang image (${docker_go}) must match go.mod go directive (${go_directive})"

echo "Go toolchain pins match: go.mod=${go_directive}, Dockerfile golang=${docker_go}"
