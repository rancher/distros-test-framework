#!/usr/bin/env bash
# Tests `setup.sh deploy` with stubbed go and systemctl in a scratch directory: bash setup_test.sh
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(mktemp -d)"
chmod 0755 "$ROOT" # as root, setup.sh validates as the releasebot user, which must reach the stage
trap 'rm -rf "$ROOT"' EXIT
PASS=0
FAIL=0
SHA_A=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
SHA_B=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb

mkdir -p "$ROOT/bin" "$ROOT/lib" "$ROOT/etc" "$ROOT/systemd"
UNIT="$ROOT/systemd/releasebot.service"
TRIAGE_UNIT="$ROOT/systemd/releasebot-triage@.service"
# go stub: `go version -m <bin>` prints the VCS stamp stored next to the fake binary.
cat > "$ROOT/bin/go" <<'EOF'
#!/usr/bin/env bash
cat "$3.vcs"
EOF
# systemctl stub (records calls): the current matrix's NORESTART fails restart, DOWN (or $ROOT/active
# not "active") the bot, BROKERDOWN the broker, which runs when $ROOT/broker exists.
cat > "$ROOT/bin/systemctl" <<EOF
#!/usr/bin/env bash
echo "\$*" >> "$ROOT/systemctl.log"
m="\$RELEASEBOT_LIB_DIR/current/matrix.yaml"
[ "\$1" = list-units ] && [ -e "$ROOT/broker" ] && echo "releasebot-triage@fmoral.service loaded active running"
[ "\$1" = restart ] && grep -qw NORESTART "\$m" && exit 1
if [ "\$1" = is-active ]; then
  case "\$3" in releasebot-triage@*) grep -qw BROKERDOWN "\$m" && exit 3; exit 0 ;; esac
  { grep -qw DOWN "\$m" || [ "\$(cat "$ROOT/active")" != active ]; } && exit 3
fi
exit 0
EOF
chmod +x "$ROOT/bin/go" "$ROOT/bin/systemctl"

# candidate <name> <revision> <modified> <matrix content>: a fake bot binary plus its matrix.
candidate() {
  local dir="$ROOT/$1"
  mkdir -p "$dir"
  cat > "$dir/releasebot" <<'EOF'
#!/usr/bin/env bash
# -validate -matrix <file>: fail on a matrix marked BAD, like a YAML that does not load.
! grep -q BAD "$3"
EOF
  chmod +x "$dir/releasebot"
  printf 'path\tgithub.com/rancher/distros-test-framework/cmd/releasebot\n\tbuild\tvcs.revision=%s\n\tbuild\tvcs.modified=%s\n' \
    "$2" "$3" > "$dir/releasebot.vcs"
  printf '%s\n' "$4" > "$dir/matrix.yaml"
}

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi
}

# release <name> <sha>: the release directory deploy creates for that candidate.
release() {
  echo "$LIB/$2-$(cat "$ROOT/$1/releasebot" "$ROOT/$1/matrix.yaml" "$HERE/triage-skill.sha256" | sha256 | cut -c1-12)"
}

LIB="$ROOT/lib"
deploy() {
  PATH="$ROOT/bin:$PATH" GO="$ROOT/bin/go" SETTLE_SECONDS=0 RELEASEBOT_LIB_DIR="$LIB" \
    RELEASEBOT_ETC_DIR="$ROOT/etc" RELEASEBOT_UNIT="$UNIT" RELEASEBOT_TRIAGE_UNIT="$TRIAGE_UNIT" \
    bash "$HERE/setup.sh" deploy "$@" 2>&1
}

check() { # name condition-exit-code output
  if [ "$2" = 0 ]; then echo "PASS: $1"; PASS=$((PASS + 1)); else echo "FAIL: $1"; echo "$3"; FAIL=$((FAIL + 1)); fi
}

reset() { # reset <is-active answer>
  : > "$ROOT/systemctl.log"
  echo "$1" > "$ROOT/active"
}

current() { readlink "$ROOT/lib/current"; }

reset active
printf 'SLACK_BOT_TOKEN=x\nRELEASEBOT_DRY_RUN=false\n' > "$ROOT/etc/releasebot.env"
chmod 0600 "$ROOT/etc/releasebot.env"
cp "$ROOT/etc/releasebot.env" "$ROOT/env.before"
touch "$ROOT/broker"
echo "# unit installed by an older setup.sh" > "$UNIT"

candidate wrongsha "$SHA_B" false "jobs: good-a"
out="$(deploy "$ROOT/wrongsha/releasebot" "$ROOT/wrongsha/matrix.yaml" "$SHA_A")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"built from $SHA_B, not $SHA_A"* ]] && [ ! -e "$ROOT/lib/current" ]
check "binary from another commit is rejected" $? "$out"

candidate dirty "$SHA_A" true "jobs: good-a"
out="$(deploy "$ROOT/dirty/releasebot" "$ROOT/dirty/matrix.yaml" "$SHA_A")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"modified tree"* ]] && [ ! -e "$ROOT/lib/current" ]
check "binary from a modified tree is rejected" $? "$out"

reset active
candidate a "$SHA_A" false "jobs: good-a"
A="$(release a "$SHA_A")"
out="$(deploy "$ROOT/a/releasebot" "$ROOT/a/matrix.yaml" "$SHA_A")"; rc=$?
[ "$rc" = 0 ] && [ "$(current)" = "$A" ] && grep -q good-a "$ROOT/lib/current/matrix.yaml" &&
  cmp -s "$UNIT" "$HERE/releasebot.service" && grep -qx "daemon-reload" "$ROOT/systemctl.log" &&
  cmp -s "$ROOT/lib/current/triage-skill.sha256" "$HERE/triage-skill.sha256" &&
  grep -qx "enable releasebot" "$ROOT/systemctl.log" && [ "$(cat "$ROOT/lib/current-commit")" = "$SHA_A" ]
check "clean build deploys, migrates the unit, enables and records the commit" $? "$out"
grep -qx "restart releasebot-triage@fmoral.service" "$ROOT/systemctl.log" && cmp -s "$TRIAGE_UNIT" "$HERE/releasebot-triage@.service"
check "a deploy installs the broker unit and moves a running broker to the new release" $? "$(cat "$ROOT/systemctl.log")"

for bad in mode link; do
  env="$ROOT/etc/releasebot.env"
  cp -p "$env" "$ROOT/env.keep"
  if [ "$bad" = mode ]; then chmod 0644 "$env"; else rm -f "$env" && ln -s "$ROOT/env.keep" "$env"; fi
  out="$(deploy "$ROOT/a/releasebot" "$ROOT/a/matrix.yaml" "$SHA_A")"; rc=$?
  [ "$rc" != 0 ] && [[ "$out" == *"must be a root-owned 0600 file"* ]]
  check "deploy refuses a tokens file that is not a 0600 regular file ($bad)" $? "$out"
  rm -f "$env" && mv "$ROOT/env.keep" "$env"
done

reset active
out="$(deploy "$ROOT/a/releasebot" "$ROOT/a/matrix.yaml" "$SHA_A")"; rc=$?
[ "$rc" = 0 ] && [ "$(current)" = "$A" ] &&
  [ "$(find "$ROOT/lib" -mindepth 1 -maxdepth 1 -name "$SHA_A-*" | wc -l | tr -d ' ')" = 1 ]
check "identical redeploy reuses its release" $? "$out"

reset active
candidate b-bad "$SHA_B" false "jobs: BAD"
out="$(deploy "$ROOT/b-bad/releasebot" "$ROOT/b-bad/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"active version is unchanged"* ]] &&
  [ "$(current)" = "$A" ] && grep -q good-a "$ROOT/lib/current/matrix.yaml" && [ ! -s "$ROOT/systemctl.log" ] &&
  ! ls -d "$ROOT/lib/$SHA_B"-* >/dev/null 2>&1 && ! ls "$ROOT/lib"/.stage.* >/dev/null 2>&1
check "invalid matrix is rejected before touching the running version" $? "$out"

reset active
echo "# previous unit" > "$UNIT"
candidate b "$SHA_B" false "jobs: good-b DOWN"
out="$(deploy "$ROOT/b/releasebot" "$ROOT/b/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"rolled back to $(basename "$A"), which is running"* ]] &&
  [ "$(current)" = "$A" ] && [ "$(cat "$ROOT/lib/current-commit")" = "$SHA_A" ] &&
  [ "$(grep -c '^restart releasebot$' "$ROOT/systemctl.log")" = 2 ] && grep -qx "# previous unit" "$UNIT"
check "a version that does not stay up is rolled back with its unit" $? "$out"
awk '/^restart releasebot$/ { r = NR } /^restart releasebot-triage@/ { b = NR } END { exit !(b > r && r > 0) }' \
  "$ROOT/systemctl.log"
check "a rollback moves the triage broker back to the previous release" $? "$(cat "$ROOT/systemctl.log")"

reset active
candidate d "$SHA_B" false "jobs: good-d BROKERDOWN"
out="$(deploy "$ROOT/d/releasebot" "$ROOT/d/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"did not stay up; rolled back to $(basename "$A"), which is running"* ]] &&
  [ "$(current)" = "$A" ] && [ "$(grep -c '^restart releasebot-triage@fmoral.service$' "$ROOT/systemctl.log")" = 2 ]
check "a release whose triage broker does not stay up is rolled back" $? "$out"

reset active
candidate c "$SHA_B" false "jobs: good-c NORESTART"
out="$(deploy "$ROOT/c/releasebot" "$ROOT/c/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"rolled back to $(basename "$A"), which is running"* ]] && [ "$(current)" = "$A" ] &&
  [ "$(grep -c '^restart releasebot$' "$ROOT/systemctl.log")" = 2 ]
check "a failing restart is rolled back too" $? "$out"

reset active
candidate a2 "$SHA_A" false "jobs: good-a2 DOWN"
out="$(deploy "$ROOT/a2/releasebot" "$ROOT/a2/matrix.yaml" "$SHA_A")"; rc=$?
[ "$rc" != 0 ] && [ "$(current)" = "$A" ] && grep -q good-a "$A/matrix.yaml" && ! grep -q good-a2 "$A/matrix.yaml" &&
  [ -d "$(release a2 "$SHA_A")" ]
check "same commit with another matrix keeps the previous release for rollback" $? "$out"

reset failed
out="$(deploy "$ROOT/b/releasebot" "$ROOT/b/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"$(basename "$A") is not fully running either"* ]] && [ "$(current)" = "$A" ]
check "a rollback that does not come up is reported, not claimed" $? "$out"

reset active
LIB="$ROOT/lib-first"; mkdir -p "$LIB"
out="$(deploy "$ROOT/b/releasebot" "$ROOT/b/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"stopped and disabled"* ]] && grep -qx "disable --now releasebot" "$ROOT/systemctl.log" &&
  [ "$(awk '/^disable --now/ { d = NR } /^daemon-reload/ { r = NR } END { print (d < r) }' "$ROOT/systemctl.log")" = 1 ]
check "a failed first deploy leaves the unit stopped and disabled" $? "$out"
grep -qx "stop releasebot-triage@\*.service" "$ROOT/systemctl.log"
check "a failed first deploy stops the triage broker too" $? "$(cat "$ROOT/systemctl.log")"

reset active
out="$(deploy "$ROOT/c/releasebot" "$ROOT/c/matrix.yaml" "$SHA_B")"; rc=$?
[ "$rc" != 0 ] && [[ "$out" == *"stopped and disabled"* ]] && [[ "$out" != *"rolled back"* ]] &&
  [ ! -e "$LIB/current" ] && [ -d "$(release b "$SHA_B")" ] && [ ! -e "$LIB/current-commit" ]
check "after a failed first deploy, the next failure never rolls back to that broken release" $? "$out"
LIB="$ROOT/lib"

mv "$ROOT/etc/releasebot.env" "$ROOT/env.moved"
out="$(deploy "$ROOT/a/releasebot" "$ROOT/a/matrix.yaml" "$SHA_A")"; rc=$?
mv "$ROOT/env.moved" "$ROOT/etc/releasebot.env"
[ "$rc" != 0 ] && [[ "$out" == *"run '"*"prepare'"* ]] && [ "$(current)" = "$A" ]
check "deploy on a host that was not prepared asks for prepare first" $? "$out"

cmp -s "$ROOT/etc/releasebot.env" "$ROOT/env.before"
check "deploys never touch the env file, so the execution mode is kept" $? ""

# prepare over an older layout (group-writable root, so entries may have been swapped), as root:
# links are removed without touching their targets and the modes are put back.
if [ "$(id -u)" = 0 ] && stat -c %h / >/dev/null 2>&1; then
  for c in useradd groupadd usermod; do printf '#!/usr/bin/env bash\nexit 0\n' > "$ROOT/bin/$c"; chmod +x "$ROOT/bin/$c"; done
  SPOOL="$ROOT/spool"
  prepare() {
    PATH="$ROOT/bin:$PATH" RELEASEBOT_LIB_DIR="$LIB" RELEASEBOT_ETC_DIR="$ROOT/etc" RELEASEBOT_UNIT="$UNIT" \
      RELEASEBOT_TRIAGE_UNIT="$ROOT/systemd/triage.service" RELEASEBOT_TRIAGE_SPOOL="$SPOOL" \
      RELEASEBOT_TRIAGE_GROUP="$(id -gn)" bash "$HERE/setup.sh" prepare nobody 2>&1
  }
  mkdir -m 0770 "$SPOOL" "$SPOOL/work" "$ROOT/elsewhere"
  printf 's' > "$ROOT/secret" && chmod 0600 "$ROOT/secret"
  ln -s "$ROOT/elsewhere" "$SPOOL/in"
  ln -s "$ROOT/secret" "$SPOOL/.broker.lock"
  out="$(prepare)"; rc=$?
  [ "$rc" = 0 ] && [ "$(stat -c %a "$ROOT/secret")" = 600 ] && [ ! -L "$SPOOL/in" ] && [ -d "$SPOOL/in" ] &&
    [ ! -L "$SPOOL/.broker.lock" ] && [ "$(stat -c '%a %U %h' "$SPOOL/.broker.lock")" = "640 nobody 1" ] &&
    [ "$(stat -c '%a %U' "$SPOOL/work")" = "700 nobody" ] && [ "$(stat -c %a "$SPOOL")" = 750 ] &&
    [ "$(stat -c %a "$SPOOL/in")" = 2770 ]
  check "prepare replaces swapped spool entries without following them" $? "$out"

  ln -f "$ROOT/secret" "$SPOOL/.broker.lock.x" && mv -f "$SPOOL/.broker.lock.x" "$SPOOL/.broker.lock"
  out="$(prepare)"; rc=$?
  [ "$rc" = 0 ] && [ "$(stat -c '%a %h' "$ROOT/secret")" = "600 1" ] &&
    [ "$(stat -c '%a %U %h' "$SPOOL/.broker.lock")" = "640 nobody 1" ]
  check "prepare replaces a hard-linked lock" $? "$out"

  rm -rf "$SPOOL/out" && : > "$SPOOL/out"
  out="$(prepare)"; rc=$?
  [ "$rc" != 0 ] && [[ "$out" == *"out is not a directory"* ]]
  check "prepare refuses a spool entry that is not a directory" $? "$out"

  rm -f "$SPOOL/out" && mkdir "$SPOOL/out" # the previous case left a file there
  env="$ROOT/etc/releasebot.env"
  chmod 0644 "$env" && cp "$env" "$ROOT/env.content"
  out="$(prepare)"; rc=$?
  [ "$rc" = 0 ] && [ "$(stat -c '%a %U' "$env")" = "600 root" ] && cmp -s "$env" "$ROOT/env.content"
  check "prepare closes an existing tokens file to root 0600 and keeps its content" $? "$out"

  mv "$env" "$ROOT/env.real" && ln -s "$ROOT/env.real" "$env"
  out="$(prepare)"; rc=$?
  [ "$rc" != 0 ] && [[ "$out" == *"is a link or not a regular file"* ]]
  check "prepare refuses a tokens file that is a link" $? "$out"
  rm -f "$env" && mv "$ROOT/env.real" "$env"
else
  echo "SKIP: prepare tests need root and GNU stat"
fi

echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" = 0 ]
