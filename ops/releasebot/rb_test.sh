#!/bin/sh
# Each rb verb runs the pinned binary through credentials.py run with the runbook's flags (stubs record
# them); a wrong word prints the usage; a failed pin leaves nothing behind and touches nothing else.
set -eu

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/pin"
# shellcheck disable=SC2016 # the stub expands them when it runs
printf '#!/bin/sh\necho "$GITHUB_TOKEN $*" >>"%s/args"\n' "$tmp" >"$tmp/pin/releasebot"
chmod +x "$tmp/pin/releasebot"
cat >"$tmp/credentials.py" <<EOF
import os, sys
if sys.argv[1] == "dtf-ref":
    open("$tmp/args", "a").write("dtf-ref " + sys.argv[2] + "\\n")
    sys.exit(0)
assert sys.argv[1:3] == ["run", "--"], sys.argv
os.environ["GITHUB_TOKEN"] = "tok"
os.execvp(sys.argv[3], sys.argv[3:])
EOF

rb() {
	RELEASEBOT_PIN=$tmp/pin RELEASEBOT_CREDENTIALS=$tmp/credentials.py RELEASEBOT_ENV_FILE=$tmp/env "$here/rb" "$@"
}

check() {
	want=$1
	shift
	: >"$tmp/args"
	rb "$@" >/dev/null
	got=$(cat "$tmp/args")
	if [ "$got" != "$want" ]; then
		echo "rb $*: ran '$got', want '$want'" >&2
		exit 1
	fi
}

P=$tmp/pin
triage="-triage-local -triage-manifest $P/triage-skill.sha256"
bumps="-matrix $P/matrix.yaml -bumps-config $P/bumps.yaml"
check "tok -message v1.37.3-rc1+k3s1 v1.37.3-rc1+rke2r1 -matrix $P/matrix.yaml" plan v1.37.3-rc1+k3s1 v1.37.3-rc1+rke2r1
check "tok -message v1.37.3-rc1+k3s1 baler -dry-run=false -matrix $P/matrix.yaml $triage" run v1.37.3-rc1+k3s1 baler
check "tok -resume -dry-run=false -matrix $P/matrix.yaml $triage" resume
check "tok -bumps $bumps -bumps-product rke2" bumps-plan rke2
check "tok -bumps -dry-run=false -poll 30s $bumps $triage" bumps
check "tok -bumps -dry-run=false -poll 30s $bumps -bumps-product rke2 -bumps-accept 11744,11746 $triage" bumps accept=11744,11746 rke2
check "tok -bumps -resume -dry-run=false -poll 30s $bumps $triage" resume-bumps
check "tok -bumps-post k3s.md -bumps-close" post k3s.md close
check "dtf-ref " dtf-ref ""

for bad in "run" "bumps k8s" "bumps accept=" "bumps accept=1,x" "bumps accept=1," "post" "post k3s.md now" "dtf-ref" "nothing"; do
	# shellcheck disable=SC2086 # each case is its words
	if rb $bad >/dev/null 2>&1; then
		echo "rb $bad: no usage error" >&2
		exit 1
	fi
done

# A commit that does not build: no pinned folder, no worktree left, and an old path left alone.
repo=$tmp/dtf
git init -q "$repo"
mkdir -p "$repo/config/releasebot" "$repo/ops/releasebot" "$tmp/home/releasebot"
touch "$repo/config/releasebot/matrix.yaml" "$repo/config/releasebot/bumps.yaml" "$repo/ops/releasebot/triage-skill.sha256"
git -C "$repo" add -A
git -C "$repo" -c user.name=t -c user.email=t@t commit -qm t
sha=$(git -C "$repo" rev-parse HEAD)
mkdir -p "$tmp/releasebot-$sha"
echo keep >"$tmp/releasebot-$sha/uncommitted"
if HOME=$tmp/home TMPDIR=$tmp RELEASEBOT_DTF_DIR=$repo "$here/rb" pin "$sha" >/dev/null 2>&1; then
	echo "rb pin: a commit without cmd/releasebot was pinned" >&2
	exit 1
fi
if [ "$(git -C "$repo" worktree list | wc -l)" -ne 1 ] || [ -n "$(ls -A "$tmp/home/releasebot")" ] ||
	[ "$(cat "$tmp/releasebot-$sha/uncommitted")" != keep ] || ls -d "$tmp"/releasebot-pin.* >/dev/null 2>&1; then
	echo "rb pin: a failed pin left something behind or touched another path" >&2
	exit 1
fi

# A branch left in the env file (rb dtf-ref) is told before bumps too.
echo "RELEASEBOT_DTF_REF=releasebot-qainfra-rc1" >"$tmp/env"
if ! rb bumps-plan rke2 2>&1 >/dev/null | grep -q "sets RELEASEBOT_DTF_REF"; then
	echo "rb bumps-plan: no warning for the env file's RELEASEBOT_DTF_REF" >&2
	exit 1
fi

echo "rb_test: ok"
