#!/bin/sh
# Each rb verb runs the pinned binary with the runbook's flags (a stub records them) and loads the env
# file; a wrong word prints the usage.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/pin"
# shellcheck disable=SC2016 # the stub expands them when it runs
printf '#!/bin/sh\necho "$GITHUB_TOKEN $*" >>"%s/args"\n' "$tmp" >"$tmp/pin/releasebot"
chmod +x "$tmp/pin/releasebot"
echo 'GITHUB_TOKEN=tok' >"$tmp/env"

check() {
	want=$1
	shift
	: >"$tmp/args"
	RELEASEBOT_PIN=$tmp/pin RELEASEBOT_ENV=$tmp/env "$here/rb" "$@" >/dev/null
	got=$(cat "$tmp/args")
	if [ "$got" != "tok $want" ]; then
		echo "rb $*: ran '$got', want 'tok $want'" >&2
		exit 1
	fi
}

P=$tmp/pin
triage="-triage-local -triage-manifest $P/triage-skill.sha256"
check "-message v1.37.3-rc1+k3s1 v1.37.3-rc1+rke2r1 -matrix $P/matrix.yaml" plan v1.37.3-rc1+k3s1 v1.37.3-rc1+rke2r1
check "-message v1.37.3-rc1+k3s1 baler -dry-run=false -matrix $P/matrix.yaml $triage" run v1.37.3-rc1+k3s1 baler
check "-resume -dry-run=false -matrix $P/matrix.yaml $triage" resume
check "-bumps -matrix $P/matrix.yaml -bumps-config $P/bumps.yaml -bumps-product rke2" bumps-plan rke2
check "-bumps -dry-run=false -poll 30s -matrix $P/matrix.yaml -bumps-config $P/bumps.yaml" bumps
check "-bumps -resume -dry-run=false -poll 30s -matrix $P/matrix.yaml -bumps-config $P/bumps.yaml" resume-bumps
check "-bumps-post k3s.md -bumps-close" post k3s.md close

for bad in "run" "bumps k8s" "post" "post k3s.md now" "nothing"; do
	# shellcheck disable=SC2086 # each case is its words
	if RELEASEBOT_PIN=$P RELEASEBOT_ENV=$tmp/env "$here/rb" $bad >/dev/null 2>&1; then
		echo "rb $bad: no usage error" >&2
		exit 1
	fi
done

echo "rb_test: ok"
