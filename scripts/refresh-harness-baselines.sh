#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

go_bin="${GO:-go}"
go_cache="${GOCACHE:-/tmp/go-build}"
baseline_dir="${HARNESS_BASELINE_DIR:-${repo_root}/baselines/harness}"
tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/mnemosyne-harness-baselines.XXXXXX")"
trap 'rm -rf "${tmp_root}"' EXIT

export GOCACHE="${go_cache}"
rm -rf "${baseline_dir}"
mkdir -p "${baseline_dir}"

for lane in smoke regression; do
  out_root="${tmp_root}/${lane}"
  mkdir -p "${out_root}"
  echo "[baseline:${lane}] generating runs into ${out_root}"
  "${go_bin}" run ./cmd/mnemosyne-harness -lane "${lane}" -out "${out_root}"
  "${go_bin}" run ./cmd/mnemosyne-harness -save-baseline "${out_root}" -baseline-dir "${baseline_dir}" -lane "${lane}"
done

echo "[baseline] refreshed at ${baseline_dir}"
