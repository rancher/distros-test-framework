#!/usr/bin/env bash
set -uo pipefail

script_dir=$(cd "$(dirname "$0")" && pwd)
audit_test_root=$(mktemp -d "${TMPDIR:-/tmp}/dtf-compare-tests.XXXXXX")
trap 'rm -rf -- "$audit_test_root"' EXIT
export AUDIT_TEST_ROOT="$audit_test_root"
passed=0
failed=0

# An exported Bash function replaces only the HTTP boundary, including pagination and status codes.
curl() {
  local destination='' url='' endpoint product tag prefix page total=2 start end status=ahead behind=0 body
  local base_sha='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
  while (($#)); do
    case "$1" in
      --output) destination="$2"; shift 2 ;;
      https://api.github.com/*) url="$1"; shift ;;
      *) shift ;;
    esac
  done
  endpoint="${url#https://api.github.com/}"
  printf '%s\n' "$endpoint" >> "$AUDIT_TEST_ROOT/calls"
  [[ "$endpoint" == repos/rancher/rke2/* ]] && product=rke2r || product=k3s
  case "$AUDIT_TEST_MODE" in
    dns)
      printf 'curl: (6) Could not resolve host: api.github.com\n' >&2
      printf '000'; return 6 ;;
    tls)
      printf 'curl: (60) SSL certificate problem: unable to get local issuer certificate\n' >&2
      printf '000'; return 60 ;;
    proxy-secret)
      printf 'curl: (5) Could not resolve proxy: qa:testtoken-never-print@proxy.invalid\n' >&2
      printf '000'; return 5 ;;
    unsafe-stderr)
      printf 'curl: (999) password=testtoken-never-print\n' >&2
      printf '000'; return 1 ;;
  esac
  if [[ "$AUDIT_TEST_MODE" == http403 ]] ||
    [[ "$AUDIT_TEST_MODE" == missing-rke2 && "$endpoint" == *rke2/commits/*-rc* ]]; then
    printf '403'
    return 22
  fi
  if [[ "$AUDIT_TEST_MODE" == invalid-json ]]; then
    printf '<html>not JSON</html>' > "$destination"
    printf '200'
    return
  fi
  if [[ "$AUDIT_TEST_MODE" == empty-json || "$AUDIT_TEST_MODE" == multiple-json ]]; then
    if [[ "$AUDIT_TEST_MODE" == empty-json ]]; then
      : > "$destination"
    else
      printf '{} {}' > "$destination"
    fi
    printf '200'
    return
  fi
  case "$endpoint" in
    */git/matching-refs/tags/*)
      prefix="${endpoint##*/}"
      if [[ "$AUDIT_TEST_MODE" == malformed-refs ]]; then
        body='[{"ref":false}]'
      elif [[ "$AUDIT_TEST_MODE" == no-ga ]]; then
        body='[]'
      else
        body=$(jq -cn --arg p "$product" --arg prefix "$prefix" '[2,9,11] |
          map($prefix+(.|tostring)+"+"+$p+"1") +
          [$prefix+"9+"+$p+"2",$prefix+"9+"+$p+"3",$prefix+"9-rc1+"+$p+"1"] |
          map({ref:("refs/tags/"+.)})')
      fi ;;
    */releases/tags/*)
      tag="${endpoint##*/}"
      tag="${tag//%2B/+}"
      if [[ "$AUDIT_TEST_MODE" == unpublished-tag && "$tag" == *9+*1 ]]; then printf '404'; return 22; fi
      if [[ "$AUDIT_TEST_MODE" == malformed-release ]]; then
        printf '{}\n' > "$destination"; printf '200'; return
      fi
      body=$(jq -cn --arg tag "$tag" --arg mode "$AUDIT_TEST_MODE" \
        '{tag_name:$tag,draft:($tag | endswith("3")),
          prerelease:(($mode == "prerelease-base") or ($tag | endswith("2")))}') ;;
    */commits/*)
      if [[ "$endpoint" == *-rc* && "$AUDIT_TEST_MODE" != identical ]]; then
        body='{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}'
      else
        body='{"sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}'
      fi ;;
    */compare/*)
      page="${endpoint##*page=}"
      case "$AUDIT_TEST_MODE" in
        commit-pages|truncated|duplicate|changed-meta) total=101 ;;
        identical) status=identical; total=0 ;;
        behind) status=behind; total=0; behind=2 ;;
        diverged) status=diverged; behind=1 ;;
        wrong-base) base_sha='cccccccccccccccccccccccccccccccccccccccc' ;;
      esac
      start=$(( (page-1)*100 ))
      end=$(( start+100 ))
      ((end <= total)) || end="$total"
      if [[ "$AUDIT_TEST_MODE" == truncated ]]; then
        if ((page == 1)); then end=1; else start=0; end=0; fi
      fi
      if [[ "$AUDIT_TEST_MODE" == duplicate && "$page" == 2 ]]; then start=0; end=1; fi
      if [[ "$AUDIT_TEST_MODE" == changed-meta && "$page" == 2 ]]; then total=102; fi
      body=$(jq -cn --arg status "$status" --arg base "$base_sha" --argjson total "$total" \
        --argjson behind "$behind" --argjson start "$start" --argjson end_index "$end" '
        {status:$status,base_commit:{sha:$base},ahead_by:$total,behind_by:$behind,total_commits:$total,
          commits:[range($start;$end_index) | . as $i |
            {sha:(("0000000000000000000000000000000000000000"+(($i+1)|tostring))[-40:]),
              commit:{message:"fix <script> and a quote\nfull body"}}]}') ;;
    *) printf '404'; return 22 ;;
  esac
  printf '%s\n' "$body" > "$destination"
  printf '200'
}
export -f curl

run() {
  local selected_baselines='{}'
  if (($# >= 3)); then selected_baselines="$3"; fi
  export AUDIT_TEST_MODE="$1"
  : > "$audit_test_root/calls"
  : > "$audit_test_root/step-summary"
  mkdir -p "$audit_test_root/report"
  printf '%s\n' '{"request_id":"STALE_REPORT","exit_code":0,"comparisons":[{"rc":"old"}]}' \
    > "$audit_test_root/report/report.json"
  printf '%s\n' 'STALE_REPORT: previous green run' > "$audit_test_root/report/summary.md"
  GH_TOKEN='testtoken-never-print' GITHUB_STEP_SUMMARY="$audit_test_root/step-summary" \
    bash "$script_dir/compare_release_commits.sh" --versions "$2" --baselines "$selected_baselines" \
      --request-id current-test --output-dir "$audit_test_root/report" > "$audit_test_root/log" 2>&1
  result=$?
}

expect_config() {
  if [[ -s "$audit_test_root/calls" ]] ||
    ! cmp -s "$audit_test_root/report/summary.md" "$audit_test_root/step-summary" ||
    grep -q 'STALE_REPORT' "$audit_test_root/report/report.json" "$audit_test_root/report/summary.md"; then
    result=99
  fi
  expect "$1" 2 '.request_id == "current-test" and .exit_code == 2 and
    (.comparisons | length) == 0 and (.findings | length) == 1 and
    .findings[0].level == "ERROR" and .findings[0].code == "CONFIG"'
}

expect() {
  local name="$1" expected="$2" query="${3:-}"
  if [[ "$result" == "$expected" ]] &&
    { [[ -z "$query" ]] || jq -e "$query" "$audit_test_root/report/report.json" >/dev/null 2>&1; }; then
    printf 'PASS: %s\n' "$name"
    passed=$((passed+1))
  else
    printf 'FAIL: %s (exit %s, expected %s)\n' "$name" "$result" "$expected"
    while IFS= read -r line; do printf '%s\n' "$line"; done < "$audit_test_root/log"
    failed=$((failed+1))
  fi
}

rc='v1.37.10-rc2+rke2r1'
run good "$rc"
expect 'numeric predecessor excludes newer, draft and prerelease releases' 0 \
  '.comparisons[0].base == "v1.37.9+rke2r1" and .comparisons[0].total_commits == 2'
if ! grep -q 'testtoken-never-print' "$audit_test_root/log" "$audit_test_root/report/report.json" \
  "$audit_test_root/report/summary.md" && grep -q '&lt;script&gt;' "$audit_test_root/step-summary"; then
  expect 'summary escapes commit HTML and no token is reported' 0
else
  result=99; expect 'summary escapes commit HTML and no token is reported' 0
fi
run good 'v1.37.9-rc1+k3s2'
expect 'same-patch lower product revision is the preceding GA' 0 '.comparisons[0].base == "v1.37.9+k3s1"'
run unpublished-tag "$rc"
expect 'tags without published releases are skipped' 0 '.comparisons[0].base == "v1.37.2+rke2r1"'
run commit-pages "$rc"
expect 'all commits, not just the first page, are reported' 0 '.comparisons[0].commits | length == 101'
for mode in truncated duplicate changed-meta wrong-base; do
  run "$mode" "$rc"
  expect "$mode comparison is never a successful partial report" 2 '.findings[0].code == "COLLECTION"'
done
for mode in identical behind diverged; do
  run "$mode" "$rc"
  expect "$mode asks for attention, with collected commits retained" 1 \
    '.comparisons | length == 1'
done
for mode in http403 invalid-json empty-json multiple-json malformed-refs malformed-release no-ga; do
  run "$mode" "$rc"
  expect "$mode fails collection, never a green empty comparison" 2 '.findings[0].level == "ERROR"'
done
run dns "$rc"
expect 'DNS failures explain HTTP 000 in the report' 2 \
  '.findings[0].message | contains("HTTP 000") and contains("curl: (6) Could not resolve host")'
run tls "$rc"
expect 'TLS failures explain HTTP 000 in the report' 2 \
  '.findings[0].message | contains("HTTP 000") and contains("curl: (60) TLS certificate verification failed")'
for mode in proxy-secret unsafe-stderr; do
  run "$mode" "$rc"
  if grep -q 'testtoken-never-print' "$audit_test_root/log" "$audit_test_root/report/report.json" \
    "$audit_test_root/report/summary.md" "$audit_test_root/step-summary"; then result=99; fi
  expect "$mode diagnostics never publish credential text" 2 '.findings[0].message | contains("curl:")'
done
run good 'v1.38.0-rc1+k3s1'
expect 'a new minor does not silently fall back to the previous minor' 2 '.findings[0].code == "BASELINE"'
run good 'v1.38.0-rc1+k3s1' '{"v1.38.0-rc1+k3s1":"v1.37.9+k3s1"}'
expect 'explicit baseline supports a .0 RC' 0 '.comparisons[0].baseline_source == "explicit"'
run prerelease-base "$rc" '{"v1.37.10-rc2+rke2r1":"v1.37.9+rke2r1"}'
expect 'an explicit baseline must be a published GA' 2 '.findings[0].code == "BASELINE"'
run missing-rke2 "$rc,v1.37.10-rc2+k3s1"
expect 'one missing RC fails the run without losing the other product report' 2 \
  '(.comparisons | length) == 1 and .comparisons[0].repo == "k3s-io/k3s" and (.requested | length) == 2'
run good "$rc,$rc,v1.37.10-rc2+k3s1"
expect 'mixed products and duplicate inputs are handled once per RC' 0 \
  '(.comparisons | length) == 2 and (.requested | length) == 2'
if grep -Eq '/issues|/pulls|/compare/(main|master)' "$audit_test_root/calls"; then
  result=99
fi
expect 'no issue, PR or main/master parity queries are made' 0
if grep -Eq '/releases\?' "$audit_test_root/calls"; then result=99; fi
expect 'GA discovery does not enumerate the unbounded release catalog' 0
for invalid in '' 'v1.37.10+rke2r1' 'v1.37.10-rc2+rke2r1,' 'v1.37.10-rc2+rke2r1/../x'; do
  run good "$invalid"
  expect_config 'invalid versions replace stale reports and populate the Actions summary'
done
run good "$rc" '{"v1.37.10-rc2+rke2r1":"v1.37.9+k3s1"}'
expect_config 'cross-product baseline is refused with fresh reports'
run good "$rc" '{"v1.37.10-rc2+rke2r1":"v1.37.9-rc1+rke2r1"}'
expect_config 'an RC baseline is refused with fresh reports'
run good "$rc" '{"v1.37.10-rc1+rke2r1":"v1.37.9+rke2r1"}'
expect_config 'unused baseline keys are refused with fresh reports'
run good "$rc" 'not-json'
expect_config 'malformed baseline JSON replaces old green artifacts with CONFIG reports'
run good "$rc" '{} {}'
expect_config 'multiple baseline objects are refused with fresh reports'
run good "$rc" ''
expect_config 'an explicitly empty CLI baseline is refused with fresh reports'

workflow="$script_dir/../.github/workflows/release-checks.yaml"
workflow_run=$(awk '
  /^      - name:/ { selected=0; running=0 }
  /^        id: commit_compare$/ { selected=1 }
  selected && /^        run: \|$/ { running=1; next }
  running && /^          / { sub(/^          /, ""); print }
' "$workflow")
if [[ -z "$workflow_run" ]]; then result=99; expect 'workflow comparison run block exists' 0; fi

# Capture only the script invocation; the real workflow shell still normalizes its inputs.
bash() { jq -cn --args '$ARGS.positional' -- "$@" > "$AUDIT_TEST_ROOT/workflow-args"; }
export -f bash
run_workflow() {
  : > "$audit_test_root/workflow-args"
  K3S_VERSIONS="$1" RKE2_VERSIONS="$2" RKE2_LTS_VERSIONS="$3" BASELINES="$4" REQUEST_ID=workflow-test \
    "$BASH" -e -o pipefail -c "$workflow_run" > "$audit_test_root/log" 2>&1
  result=$?
}
run_workflow 'v1.37.1-rc2+k3s1' '' '' ''
if ! jq -e '.[4] == "{}" and .[6] == "workflow-test"' "$audit_test_root/workflow-args" >/dev/null; then result=99; fi
expect 'blank workflow baseline becomes the JSON default' 0
run_workflow 'v1.37.0+k3s1' '' '' ''
if [[ -s "$audit_test_root/workflow-args" ]]; then result=99; fi
expect 'GA-only workflow inputs do not call the comparison script' 0
run_workflow 'v1.37.1-rc2+k3s1' 'v1.36.5-rc2+rke2r1' 'v1.34.12-rc2+rke2r1' '{"v1.37.1-rc2+k3s1":"v1.37.0+k3s1"}'
if ! jq -e '.[2] == "v1.37.1-rc2+k3s1,v1.36.5-rc2+rke2r1,v1.34.12-rc2+rke2r1" and
  .[4] == "{\"v1.37.1-rc2+k3s1\":\"v1.37.0+k3s1\"}"' "$audit_test_root/workflow-args" >/dev/null; then result=99; fi
expect 'workflow combines all three RC inputs and preserves explicit baselines' 0
if grep -Eq '^[[:space:]]*run:.*\$\{\{.*inputs\.' "$workflow"; then result=99; fi
expect 'artifact checks never interpolate workflow inputs into shell source' 0
printf '\nPASS=%s FAIL=%s\n' "$passed" "$failed"
((failed == 0))
