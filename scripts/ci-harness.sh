#!/usr/bin/env bash
set -euo pipefail

lane="${1:-}"
if [[ -z "${lane}" ]]; then
  echo "usage: $0 <smoke|regression>" >&2
  exit 1
fi

case "${lane}" in
  smoke|regression) ;;
  *)
    echo "unsupported lane: ${lane}" >&2
    exit 1
    ;;
esac

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

go_bin="${GO:-go}"
go_cache="${GOCACHE:-/tmp/go-build}"
baseline_dir="${HARNESS_BASELINE_DIR:-${repo_root}/baselines/harness}"
out_root="${HARNESS_OUT_ROOT:-$(mktemp -d "${TMPDIR:-/tmp}/mnemosyne-harness-${lane}.XXXXXX")}"

mkdir -p "${out_root}"
export GOCACHE="${go_cache}"

echo "[harness:${lane}] output root: ${out_root}"
"${go_bin}" run ./cmd/mnemosyne-harness -lane "${lane}" -out "${out_root}"
"${go_bin}" run ./cmd/mnemosyne-harness -rollup "${out_root}" -lane "${lane}" -rollup-json "${out_root}/rollup-${lane}.json"
"${go_bin}" run ./cmd/mnemosyne-harness -check-baseline "${out_root}" -baseline-dir "${baseline_dir}" -lane "${lane}"

echo "[harness:${lane}] baseline check passed"
echo "[harness:${lane}] reports: ${out_root}"
echo "[harness:${lane}] gocache: ${go_cache}"
