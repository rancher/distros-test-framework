#!/bin/sh
# Tests the rke2-packaging tag lookup in release_checks.sh with a stubbed curl: sh release_checks_test.sh
set -u

script="$(cd "$(dirname "$0")" && pwd)/release_checks.sh"
root="$(mktemp -d)"
trap 'rm -rf "$root"' EXIT
pass=0
fail=0

# curl stub: refs from $root/refs.json; 60 assets for the tag in $root/good-tag, else 3.
# $root/fail-refs or $root/fail-release makes that call fail like curl -f on an HTTP error.
mkdir -p "$root/bin"
cat > "$root/bin/curl" <<EOF
#!/bin/sh
for url; do :; done
echo "\$url" >> "$root/curl.log"
fail() { echo "curl: (22) The requested URL returned error: \$1" >&2; exit 22; }
case "\$url" in
  */git/matching-refs/tags/*) [ -e "$root/fail-refs" ] && fail 403 ;;
  */releases/tags/*) [ -e "$root/fail-release" ] && fail 502 ;;
esac
case "\$url" in
  */git/matching-refs/tags/*) cat "$root/refs.json" ;;
  */releases/tags/\$(sed 's/+/%2B/g' "$root/good-tag")) echo '{"assets":[$(awk 'BEGIN { for (i = 1; i <= 60; i++) printf "%s%d", (i > 1 ? "," : ""), i }')]}' ;;
  *) echo '{"assets":[1,2,3]}' ;;
esac
EOF
chmod +x "$root/bin/curl"
PATH="$root/bin:$PATH" # every check below runs against the stub

refs() { # refs <tag>...: the matching-refs answer for these tags, in the given order
  printf '[' > "$root/refs.json"
  sep=""
  for t; do printf '%s{"ref":"refs/tags/%s"}' "$sep" "$t" >> "$root/refs.json"; sep=","; done
  printf ']\n' >> "$root/refs.json"
}

lookup() { # lookup <version>: runs the packaging check and prints its output
  (
    version="$1"
    cd "$root" || exit 1
    set +u -- -v "$version"
    # shellcheck disable=SC2034 # both are read by the sourced script
    RELEASE_CHECKS_FUNCTIONS_ONLY=1 VERSION="$version"
    # shellcheck source=/dev/null
    . "$script"
    verify_asset_count_rke2_packaging
  ) 2>&1
}

check() { # check <name> <condition-exit-code> <output>
  if [ "$2" = 0 ]; then echo "PASS: $1"; pass=$((pass + 1)); else echo "FAIL: $1"; echo "$3"; fail=$((fail + 1)); fi
}

v="v1.37.1-rc2+rke2r1"

refs "$v.testing.2" "$v.testing.10" "$v.testing.1"
echo "$v.testing.10" > "$root/good-tag"
out="$(lookup "$v")"
case "$out" in *"PASS: RKE2 packaging assets ($v.testing.10)"*) true ;; *) false ;; esac
check "highest .testing.N wins numerically, whatever the order" $? "$out"

refs "$v.testing.0"
echo "$v.testing.0" > "$root/good-tag"
out="$(lookup "$v")"
case "$out" in *"PASS: RKE2 packaging assets ($v.testing.0)"*) true ;; *) false ;; esac
check "a single rebuild-less tag is found" $? "$out"

refs "$v.testing.3-extra" "$v.testingx"
out="$(lookup "$v")"
case "$out" in *"packaging release tag not found"*) true ;; *) false ;; esac
check "tags without a numeric .testing.N suffix are ignored" $? "$out"

refs
out="$(lookup "$v")"
case "$out" in *"packaging release tag not found"*) true ;; *) false ;; esac
check "an empty list means the tag does not exist" $? "$out"

touch "$root/fail-refs"
out="$(lookup "$v")"
rm "$root/fail-refs"
case "$out" in *"GitHub API lookup failed for the packaging tags"*) true ;; *) false ;; esac
check "an HTTP error on the tag lookup is a failed lookup, not a missing tag" $? "$out"

for body in '{"message":"Git Repository is empty."}' '<html>502 Bad Gateway</html>'; do
  printf '%s\n' "$body" > "$root/refs.json"
  out="$(lookup "$v")"
  case "$out" in *"GitHub API lookup failed for the packaging tags"*) true ;; *) false ;; esac
  check "a non-list answer ($body) is a failed lookup" $? "$out"
done

refs "$v.testing.1"
touch "$root/fail-release"
out="$(lookup "$v")"
rm "$root/fail-release"
case "$out" in *"GitHub API lookup failed for release $v.testing.1"*) true ;; *) false ;; esac
check "an HTTP error on the release lookup is a failed lookup, not 0 assets" $? "$out"

# A longer version sharing the prefix (rke2r10 for rke2r1) is never taken for this one.
refs "$v.testing.2" "${v}0.testing.9"
echo "$v.testing.2" > "$root/good-tag"
out="$(lookup "$v")"
case "$out" in *"PASS: RKE2 packaging assets ($v.testing.2)"*) true ;; *) false ;; esac
check "a tag of a longer version with the same prefix is ignored" $? "$out"

# A version that is not a release tag never reaches a URL: no curl call, and the run fails.
: > "$root/curl.log"
out=$(cd "$root" && sh "$script" -v 'v1.37.1+rke2r1/../x' 2>&1)
rc=$?
[ "$rc" = 1 ] && printf '%s' "$out" | grep -q "not a release tag" && [ ! -s "$root/curl.log" ]
check "a version that is not a release tag is refused before any request" $? "$out"

echo "PASS=$pass FAIL=$fail"
[ "$fail" = 0 ]
