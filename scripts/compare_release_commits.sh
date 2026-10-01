#!/usr/bin/env bash
set -euo pipefail

usage() {
  printf '%s\n' 'Usage: bash scripts/compare_release_commits.sh --versions <comma-separated RC tags>' \
    '       [--baselines <JSON object mapping RC tags to GA tags>]' \
    '       [--output-dir <directory>] [--request-id <id>]' \
    'Compares the preceding published GA with each RC. Does not validate issues or backports.' \
    'Exit: 0 complete/ahead; 1 attention (identical/behind/diverged); 2 collection/configuration error.'
}

config_error() { printf 'ERROR [CONFIG] %s\n' "$1" >&2; exit 2; }

versions=''
baselines='{}'
output_dir='tmp/release-commits'
request_id=''
while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --versions|--baselines|--output-dir|--request-id)
      (($# >= 2)) || config_error "missing value for $1"
      case "$1" in
        --versions) versions="$2" ;;
        --baselines) baselines="$2" ;;
        --output-dir) output_dir="$2" ;;
        --request-id) request_id="$2" ;;
      esac
      shift 2 ;;
    *) config_error 'unknown option; use --help' ;;
  esac
done
for dependency in curl jq; do
  command -v "$dependency" >/dev/null || config_error "install $dependency first"
done
[[ -n "$versions" && -n "$output_dir" ]] || config_error '--versions and a nonempty output directory are required'
[[ "$request_id" =~ ^[a-zA-Z0-9._-]{0,80}$ ]] || config_error 'request id must be at most 80 letters/digits/._-'

rc_pattern='^v[0-9]+\.[0-9]+\.[0-9]+-rc[1-9][0-9]*\+(k3s|rke2r)[1-9][0-9]*$'
ga_pattern='^v[0-9]+\.[0-9]+\.[0-9]+\+(k3s|rke2r)[1-9][0-9]*$'
tags=$(jq -cn --arg versions "$versions" '$versions | split(",") | map(gsub("^\\s+|\\s+$"; "")) | unique')
while IFS= read -r tag; do
  [[ "$tag" =~ $rc_pattern ]] || config_error 'every version must be a full K3s/RKE2 RC tag (no empty entries)'
done < <(jq -r '.[]' <<< "$tags")
if ! jq -se --argjson tags "$tags" --arg rc "$rc_pattern" --arg ga "$ga_pattern" '
  length == 1 and (.[0] | type == "object" and all(to_entries[];
    (.key | test($rc)) and (.value | type == "string" and test($ga)) and
    (.key as $key | $tags | index($key) != null)))
' <<< "$baselines" >/dev/null 2>&1; then
  config_error '--baselines must map requested RC tags to full GA tags'
fi
while IFS=$'\t' read -r rc base; do
  [[ "${rc##*+}" =~ ^k3s ]] && product=k3s || product=rke2r
  [[ "${base##*+}" =~ ^$product ]] || config_error 'a baseline must belong to the same product as its RC'
done < <(jq -r 'to_entries[] | [.key,.value] | @tsv' <<< "$baselines")

mkdir -p -- "$output_dir" || config_error 'cannot create output directory'
for name in report.json summary.md; do
  [[ ! -L "$output_dir/$name" ]] || config_error 'report paths must not be symlinks'
done
umask 077
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/dtf-compare-commits.XXXXXX")
trap 'rm -rf -- "$work_dir"' EXIT
: > "$work_dir/comparisons.jsonl"
: > "$work_dir/findings.jsonl"
headers=(--header 'Accept: application/vnd.github+json' --header 'X-GitHub-Api-Version: 2022-11-28')
token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
if [[ -n "$token" ]]; then
  [[ "$token" != *$'\n'* && "$token" != *$'\r'* ]] || config_error 'token must not contain line breaks'
  # A header file keeps the token out of curl's process arguments and the report.
  printf 'Authorization: Bearer %s\n' "$token" > "$work_dir/auth-header"
  headers+=(--header "@$work_dir/auth-header")
fi
unset token

finding() {
  local level="$1" code="$2" rc="$3" message="$4"
  printf '%s [%s] %s: %s\n' "$level" "$code" "$rc" "$message"
  jq -cn --arg level "$level" --arg code "$code" --arg rc "$rc" --arg message "$message" \
    '{level:$level,code:$code,rc:$rc,message:$message}' >> "$work_dir/findings.jsonl"
}

api() {
  local endpoint="$1" destination="$2" rc="$3" allow_missing="${4:-false}" status
  if ! status=$(curl --disable --silent --show-error --fail --connect-timeout 10 --max-time 45 \
    --retry 2 --retry-delay 1 --retry-max-time 90 "${headers[@]}" \
    --output "$destination" --write-out '%{http_code}' "https://api.github.com/$endpoint" \
    2> "$work_dir/curl-error"); then
    [[ "$status" != 404 || "$allow_missing" != true ]] || return 4
    finding ERROR COLLECTION "$rc" "GitHub request failed (HTTP ${status:-000}): $endpoint"
    return 1
  fi
  if [[ "$status" != 200 ]] || ! jq -se 'length == 1' "$destination" >/dev/null 2>&1; then
    finding ERROR COLLECTION "$rc" "unexpected HTTP $status or invalid JSON: $endpoint"
    return 1
  fi
}

find_base() {
  local repo="$1" rc="$2" product="$3" prefix refs candidate encoded response result
  prefix="${rc%.*}."
  refs="$work_dir/$product.$prefix.refs"
  if [[ ! -f "$refs" ]]; then
    api "repos/$repo/git/matching-refs/tags/$prefix" "$refs" "$rc" || return 1
  fi
  if ! jq -e --arg prefix "refs/tags/$prefix" 'type == "array" and all(.[];
    (.ref | type == "string" and startswith($prefix)))' "$refs" >/dev/null; then
    finding ERROR COLLECTION "$rc" 'malformed matching-tags response'
    return 1
  fi
  # Limit discovery to this minor; enumerating all releases can hit GitHub's pagination cap.
  jq -r --arg rc "$rc" --arg ga "$ga_pattern" '
    def version:
      capture("^v(?<major>[0-9]+)\\.(?<minor>[0-9]+)\\.(?<patch>[0-9]+)(?:-rc[0-9]+)?\\+(?<product>k3s|rke2r)(?<rev>[0-9]+)$")
      | .major |= tonumber | .minor |= tonumber | .patch |= tonumber | .rev |= tonumber;
    ($rc | version) as $target |
    map(.ref | ltrimstr("refs/tags/") | select(test($ga)) |
      . as $tag | version as $v |
      select($v.major == $target.major and $v.minor == $target.minor and $v.product == $target.product and
        [$v.patch,$v.rev] < [$target.patch,$target.rev]) |
      {tag:$tag,patch:$v.patch,rev:$v.rev}) |
    sort_by([.patch,.rev]) | reverse | .[].tag
  ' "$refs" > "$work_dir/candidates" || return 1
  while IFS= read -r candidate; do
    encoded=$(jq -rn --arg tag "$candidate" '$tag | @uri')
    response="$work_dir/base-release.json"
    if api "repos/$repo/releases/tags/$encoded" "$response" "$rc" true; then
      if ! jq -e --arg tag "$candidate" '.tag_name == $tag and
        (.draft | type == "boolean") and (.prerelease | type == "boolean")' "$response" >/dev/null; then
        finding ERROR COLLECTION "$rc" 'malformed GA release response'
        return 1
      fi
      if jq -e '.draft == false and .prerelease == false' "$response" >/dev/null; then
        printf '%s\n' "$candidate" > "$work_dir/selected-base"
        return 0
      fi
    else
      result=$?
      [[ "$result" == 4 ]] || return 1
    fi
  done < "$work_dir/candidates"
  finding ERROR BASELINE "$rc" 'no preceding published GA in this minor; supply --baselines explicitly'
  return 1
}

resolve_tag() {
  local repo="$1" tag="$2" destination="$3" rc="$4" encoded
  encoded=$(jq -rn --arg tag "$tag" '$tag | @uri')
  api "repos/$repo/commits/$encoded" "$work_dir/ref.json" "$rc" || return 1
  if ! jq -e '.sha | type == "string" and test("^[0-9a-f]{40}$")' "$work_dir/ref.json" >/dev/null; then
    finding ERROR COLLECTION "$rc" "cannot resolve tag $tag to a commit SHA"
    return 1
  fi
  jq -r '.sha' "$work_dir/ref.json" > "$destination"
}

compare_rc() {
  local rc="$1" repo product base base_sha head_sha page count total status encoded source
  if [[ "${rc##*+}" =~ ^k3s ]]; then
    product=k3s
    repo=k3s-io/k3s
  else
    product=rke2
    repo=rancher/rke2
  fi
  base=$(jq -r --arg rc "$rc" '.[$rc] // empty' <<< "$baselines")
  source=explicit
  if [[ -z "$base" ]]; then
    source=automatic
    find_base "$repo" "$rc" "$product" || return 1
    base=$(< "$work_dir/selected-base")
  else
    encoded=$(jq -rn --arg tag "$base" '$tag | @uri')
    api "repos/$repo/releases/tags/$encoded" "$work_dir/base-release.json" "$rc" || return 1
    if ! jq -e --arg base "$base" '.tag_name == $base and .draft == false and .prerelease == false' \
      "$work_dir/base-release.json" >/dev/null; then
      finding ERROR BASELINE "$rc" "explicit baseline $base is not a published GA"
      return 1
    fi
  fi
  resolve_tag "$repo" "$base" "$work_dir/base-sha" "$rc" || return 1
  resolve_tag "$repo" "$rc" "$work_dir/head-sha" "$rc" || return 1
  base_sha=$(< "$work_dir/base-sha")
  head_sha=$(< "$work_dir/head-sha")
  : > "$work_dir/commits.jsonl"
  for ((page=1; page<=1000; page++)); do
    api "repos/$repo/compare/$base_sha...$head_sha?per_page=100&page=$page" "$work_dir/compare.json" "$rc" || return 1
    if ! jq -e --arg base "$base_sha" '
      (.base_commit.sha == $base) and (.status | IN("ahead","behind","diverged","identical")) and
      all([.ahead_by,.behind_by,.total_commits][]; type == "number" and . >= 0 and floor == .) and
      (.commits | type == "array" and all(.[];
        (.sha | type == "string" and test("^[0-9a-f]{40}$")) and (.commit.message | type == "string")))
    ' "$work_dir/compare.json" >/dev/null; then
      finding ERROR COLLECTION "$rc" 'malformed compare response or unexpected base SHA'
      return 1
    fi
    if ((page == 1)); then
      jq '{status,ahead_by,behind_by,total_commits}' "$work_dir/compare.json" > "$work_dir/comparison-meta.json"
    elif ! jq -e --slurpfile meta "$work_dir/comparison-meta.json" \
      '{status,ahead_by,behind_by,total_commits} == $meta[0]' "$work_dir/compare.json" >/dev/null; then
      finding ERROR COLLECTION "$rc" 'comparison metadata changed between pages'
      return 1
    fi
    jq -c --arg repo "$repo" '.commits[] | {sha,
      subject:(.commit.message | split("\n")[0] | gsub("[\u0000-\u001f\u007f]"; "")),
      url:("https://github.com/"+$repo+"/commit/"+.sha)}' "$work_dir/compare.json" >> "$work_dir/commits.jsonl"
    count=$(jq -s 'length' "$work_dir/commits.jsonl")
    total=$(jq '.total_commits' "$work_dir/comparison-meta.json")
    if ((count >= total)); then break; fi
    if [[ $(jq '.commits | length' "$work_dir/compare.json") == 0 ]]; then break; fi
  done
  if ! jq -se --argjson total "$total" 'length == $total and (unique_by(.sha) | length) == $total' \
    "$work_dir/commits.jsonl" >/dev/null; then
    finding ERROR COLLECTION "$rc" "incomplete/duplicate commits: collected $count, expected $total"
    return 1
  fi
  jq -cn --arg repo "$repo" --arg rc "$rc" --arg base "$base" --arg source "$source" \
    --arg base_sha "$base_sha" --arg head_sha "$head_sha" --slurpfile meta "$work_dir/comparison-meta.json" \
    --slurpfile commits "$work_dir/commits.jsonl" '$meta[0] + {repo:$repo,rc:$rc,base:$base,baseline_source:$source,
      base_sha:$base_sha,head_sha:$head_sha,commits:$commits,
      compare_url:("https://github.com/"+$repo+"/compare/"+$base+"..."+$rc),
      pinned_compare_url:("https://github.com/"+$repo+"/compare/"+$base_sha+"..."+$head_sha)}' \
    >> "$work_dir/comparisons.jsonl"
  status=$(jq -r '.status' "$work_dir/comparison-meta.json")
  printf '\n%s → %s: %s, %s new commits\nhttps://github.com/%s/compare/%s...%s\n' "$base" "$rc" "$status" "$total" "$repo" "$base" "$rc"
  jq -rs '.[] | "  \(.sha[:12]) \(.subject)\n  \(.url)"' "$work_dir/commits.jsonl"
  if [[ "$status" != ahead ]] || ((total == 0)); then
    finding ATTENTION COMPARISON "$rc" "GA $base → RC is $status (not ahead with new commits); review the baseline/release"
  fi
}

while IFS= read -r rc; do
  if ! compare_rc "$rc"; then
    # A failed RC never prevents the other reports or silently turns an unhandled error green.
    if ! jq -se --arg rc "$rc" 'any(.[]; .rc == $rc and .level == "ERROR")' \
      "$work_dir/findings.jsonl" >/dev/null; then
      finding ERROR COLLECTION "$rc" 'comparison could not be completed'
    fi
  fi
done < <(jq -r '.[]' <<< "$tags")
jq -n --arg request_id "$request_id" --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --argjson requested "$tags" --slurpfile comparisons "$work_dir/comparisons.jsonl" \
  --slurpfile findings "$work_dir/findings.jsonl" '
  {schema_version:1,request_id:$request_id,generated_at:$generated_at,requested:$requested,
    comparisons:$comparisons,findings:$findings,
    exit_code:(if any($findings[]; .level == "ERROR") then 2
      elif any($findings[]; .level == "ATTENTION") then 1 else 0 end)}
' > "$output_dir/report.json"
jq -r '
  "# GA → RC commit comparison\n",
  "Collection/comparison only: no issue-status or cross-minor backport validation.\n",
  "Exit code: \(.exit_code). Compared \(.comparisons|length)/\(.requested|length) requested RCs.\n",
  (.findings[] | "- **\(.level) [\(.code)]** \(.rc): \(.message | @html)"),
  (.comparisons[] | "\n## \(.rc)\n",
    "GA: `\(.base)` (\(.baseline_source)). Status: **\(.status)**. New commits: **\(.total_commits)**.\n",
    "[Tag comparison](\(.compare_url)) · [Pinned comparison](\(.pinned_compare_url))\n",
    "Base SHA: `\(.base_sha)`; RC SHA: `\(.head_sha)`.\n",
    "<details><summary>Commits</summary>\n<ul>",
    (.commits[] | "<li><a href=\"\(.url)\"><code>\(.sha[:12])</code></a> \(.subject | @html)</li>"),
    "</ul>\n</details>\n")
' "$output_dir/report.json" > "$output_dir/summary.md"
if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
  # The summary is readable on failed jobs too, without uploading any temporary credential files.
  while IFS= read -r line; do printf '%s\n' "$line"; done < "$output_dir/summary.md" >> "$GITHUB_STEP_SUMMARY"
fi
exit_code=$(jq '.exit_code' "$output_dir/report.json")
printf '\nResult: exit %s. Reports: %s/report.json and %s/summary.md\n' "$exit_code" "$output_dir" "$output_dir"
exit "$exit_code"
