#!/bin/bash
set -euo pipefail

repo_root=$(cd -- "$(dirname "$0")/.." && pwd -P)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/dtf-shell-test.XXXXXX")
trap 'rm -rf "$test_root"' EXIT

mkdir -p "$test_root/artifacts" "$test_root/report" "$test_root/bin"
asset_requests="$test_root/asset-requests"
(
  cd "$test_root/artifacts"
  # Load the real helpers without invoking the infrastructure-facing main.
  # shellcheck source=/dev/null
  source <(sed '$d' "$repo_root/infrastructure/legacy/airgap/setup/get_artifacts.sh") \
    rke2 v1.32.13+rke2r1 linux amd64 'https://prime.example.invalid/path with spaces' '' tar.gz

  # Called indirectly by the sourced artifact helpers.
  # shellcheck disable=SC2317
  download_retry() {
    [[ $# -eq 2 && "$1" == wget ]]
    printf '%s\n' "$2" >> "$asset_requests"
  }
  # shellcheck disable=SC2317
  safe_download_install() { :; }
  # shellcheck disable=SC2317
  verify_checksums() { :; }

  get_assets
  if registry_url=https://unsupported.example.invalid get_assets; then
    echo 'Unsupported artifact source was accepted' >&2
    exit 1
  fi
)
grep -Fxq 'https://prime.example.invalid/path with spaces/rke2/v1.32.13+rke2r1/rke2.linux-amd64.tar.gz' "$asset_requests"
test "$(wc -l < "$asset_requests")" -eq 4

if bash "$repo_root/scripts/delete_resources.sh" -h > "$test_root/help" 2>&1; then
  echo 'Cleanup help changed its exit status' >&2
  exit 1
fi
grep -Fq '"dsf-<name>-"' "$test_root/help"
grep -Fq '"dsf-<RESOURCE_NAME>-"' "$test_root/help"
if grep -q 'No such file or directory' "$test_root/help"; then
  echo 'Cleanup help interpreted its text as shell redirections' >&2
  exit 1
fi

cat > "$test_root/bin/go" <<'EOF'
#!/bin/bash
set -eu
cat > processreport <<'REPORT'
#!/bin/bash
printf '%s\n' "$@" > "$REPORT_ARGS"
REPORT
chmod +x processreport
EOF
chmod +x "$test_root/bin/go"
touch -t 202001010000 "$test_root/report/k3s_old.log"
touch -t 202001020000 "$test_root/report/rke2_latest run.log"
(
  cd "$test_root"
  PATH="$test_root/bin:$PATH" REPORT_ARGS="$test_root/report-args" \
    bash "$repo_root/scripts/e2e_report.sh" > "$test_root/report-output" 2>&1
)
printf '%s\n' '-f' './report/rke2_latest run.log' '-p' rke2 > "$test_root/expected-args"
cmp "$test_root/expected-args" "$test_root/report-args"

echo 'Shell regressions passed'
