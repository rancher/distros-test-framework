#!/usr/bin/env bash
# Run as root on the bot host. `prepare [triage-user]` creates the user, directories, spool and units;
# `deploy` installs a binary built from a reviewed commit plus its matrix, root-owned, read-only.
set -euo pipefail

BOT_USER=releasebot
LIB_DIR="${RELEASEBOT_LIB_DIR:-/usr/local/lib/releasebot}"
ETC_DIR="${RELEASEBOT_ETC_DIR:-/etc/releasebot}"
UNIT="${RELEASEBOT_UNIT:-/etc/systemd/system/releasebot.service}"
TRIAGE_UNIT="${RELEASEBOT_TRIAGE_UNIT:-/etc/systemd/system/releasebot-triage@.service}"
TRIAGE_GROUP="${RELEASEBOT_TRIAGE_GROUP:-releasebot-triage}"
SPOOL="${RELEASEBOT_TRIAGE_SPOOL:-/var/lib/releasebot-triage}"
GO="${GO:-$(command -v go || echo /usr/local/go/bin/go)}"
SETTLE_SECONDS="${SETTLE_SECONDS:-10}"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

die() { echo "error: $*" >&2; exit 1; }

prepare() {
  if ! getent passwd "$BOT_USER" >/dev/null; then
    # No login shell, no home, no extra groups (no sudo, no docker), no SSH keys.
    useradd --system --user-group --no-create-home --home-dir /var/lib/releasebot \
      --shell /usr/sbin/nologin "$BOT_USER"
  fi

  install -d -o root -g root -m 0755 "$LIB_DIR" "$ETC_DIR"
  local env="$ETC_DIR/releasebot.env"
  if [ -L "$env" ] || { [ -e "$env" ] && [ ! -f "$env" ]; }; then
    die "$env is a link or not a regular file; replace it with the file itself"
  elif [ -e "$env" ]; then
    chown root:root "$env" && chmod 0600 "$env" # holds every token: kept, but closed to others
  else
    install -o root -g root -m 0600 "$HERE/releasebot.env.example" "$env"
    echo "fill in $env (root:root 0600) before starting the service"
  fi

  # The bot and the triage broker (run as triage-user, who owns the claude-sandbox) share a spool
  # through one group; the bot asks a person at once when no broker is running.
  getent group "$TRIAGE_GROUP" >/dev/null || groupadd --system "$TRIAGE_GROUP"
  usermod -a -G "$TRIAGE_GROUP" "$BOT_USER"
  # The root is not group-writable, so neither side can swap in/, work/ or out/ for a symlink;
  # in/ and out/ are shared, work/ is the broker's own.
  [ ! -L "$SPOOL" ] || die "$SPOOL is a symlink; remove it and run prepare again"
  install -d -o root -g "$TRIAGE_GROUP" -m 0750 "$SPOOL"
  # An older layout had a group-writable root, so its entries may have been swapped: links are
  # removed (never followed) and anything but a directory is refused.
  local d
  for d in in out work; do
    if [ -L "$SPOOL/$d" ]; then rm -f "$SPOOL/$d"; fi
    [ ! -e "$SPOOL/$d" ] || [ -d "$SPOOL/$d" ] || die "$SPOOL/$d is not a directory; remove it and run prepare again"
  done
  install -d -o root -g "$TRIAGE_GROUP" -m 2770 "$SPOOL/in" "$SPOOL/out"
  if [ -n "${1:-}" ]; then
    usermod -a -G "$TRIAGE_GROUP" "$1"
    install -d -o "$1" -g "$TRIAGE_GROUP" -m 0700 "$SPOOL/work"
    # The broker cannot create files in the root: its lock (group-readable, see brokerRunning) is made
    # here, fresh unless it already is a single-link regular file of that user.
    local lock="$SPOOL/.broker.lock"
    if [ -L "$lock" ] || [ ! -f "$lock" ] || [ "$(stat -c '%h %U' "$lock")" != "1 $1" ]; then
      rm -f "$lock"
      install -o "$1" -g "$TRIAGE_GROUP" -m 0640 /dev/null "$lock"
    else
      chgrp "$TRIAGE_GROUP" "$lock" && chmod 0640 "$lock"
    fi
  fi

  install -o root -g root -m 0644 "$HERE/releasebot.service" "$UNIT"
  install -o root -g root -m 0644 "$HERE/releasebot-triage@.service" "$TRIAGE_UNIT"
  systemctl daemon-reload
  if [ -n "${1:-}" ]; then
    echo "triage broker: put the pinned skill in ~$1/releasebot-triage/skill, then enable releasebot-triage@$1"
  fi
}

# verify_build checks the VCS stamp Go embeds in the binary: the exact commit, from a clean tree.
verify_build() {
  local bin="$1" sha="$2" info rev modified
  info="$("$GO" version -m "$bin")" || die "cannot read build info from $bin"
  rev="$(awk '$1 == "build" && $2 ~ /^vcs\.revision=/ { sub(/^vcs\.revision=/, "", $2); print $2 }' <<<"$info")"
  modified="$(awk '$1 == "build" && $2 ~ /^vcs\.modified=/ { sub(/^vcs\.modified=/, "", $2); print $2 }' <<<"$info")"
  [ "$rev" = "$sha" ] || die "binary was built from ${rev:-an unknown revision}, not $sha"
  [ "$modified" = "false" ] || die "binary was built from a modified tree (vcs.modified=${modified:-unknown})"
}

# as_bot runs a command as the bot user when possible, so validation never runs with root rights.
as_bot() {
  if command -v runuser >/dev/null && getent passwd "$BOT_USER" >/dev/null; then
    runuser -u "$BOT_USER" -- "$@"
  else
    "$@"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum; else shasum -a 256; fi
}

# start_healthy restarts the unit and reports whether it stayed up; never exits under set -e.
start_healthy() {
  systemctl restart releasebot || return 1
  sleep "$SETTLE_SECONDS"
  systemctl is-active --quiet releasebot
}

# deploy <binary> <matrix.yaml> <commit-sha>: a release is <sha>-<hash of binary+matrix+manifest>,
# never overwritten, so the previous one survives for rollback even when only the matrix changes.
deploy() {
  local bin="$1" matrix="$2" sha="$3" stage release prev prev_unit prev_triage brokers
  [[ "$sha" =~ ^[0-9a-f]{40}$ ]] || die "commit sha must be 40 hex characters"
  if [ ! -d "$LIB_DIR" ] || [ ! -f "$ETC_DIR/releasebot.env" ]; then
    die "host not prepared: run '$0 prepare' and fill $ETC_DIR/releasebot.env first"
  fi
  if [ -L "$ETC_DIR/releasebot.env" ] || [ "$(owner_mode "$ETC_DIR/releasebot.env")" != "$(id -u) 600" ]; then
    die "$ETC_DIR/releasebot.env holds the tokens and must be a root-owned 0600 file; run '$0 prepare'"
  fi
  verify_build "$bin" "$sha"

  # Stage and validate the candidate before touching the active version.
  stage="$(mktemp -d "$LIB_DIR/.stage.XXXXXX")"
  trap 'rm -rf "$stage"' EXIT
  install -m 0755 "$bin" "$stage/releasebot"
  install -m 0644 "$matrix" "$stage/matrix.yaml"
  # The broker checks the triage skill against this manifest before every triage.
  install -m 0644 "$HERE/triage-skill.sha256" "$stage/triage-skill.sha256"
  chmod 0755 "$stage"
  as_bot "$stage/releasebot" -validate -matrix "$stage/matrix.yaml" ||
    die "candidate rejected: its matrix does not load; the active version is unchanged"

  release="$sha-$(cat "$stage/releasebot" "$stage/matrix.yaml" "$stage/triage-skill.sha256" | sha256 | cut -c1-12)"
  if [ -d "$LIB_DIR/$release" ]; then
    rm -rf "$stage" # identical release already installed
  else
    mv "$stage" "$LIB_DIR/$release"
  fi
  trap - EXIT

  # The units ship with the release; the execution mode lives in the env file and is kept.
  prev="$(readlink "$LIB_DIR/current" || true)"
  prev_unit="$(save_unit "$UNIT")"
  prev_triage="$(save_unit "$TRIAGE_UNIT")"
  brokers="$(active_brokers)"

  install -m 0644 "$HERE/releasebot.service" "$UNIT"
  install -m 0644 "$HERE/releasebot-triage@.service" "$TRIAGE_UNIT"
  if ! { systemctl daemon-reload && systemctl enable releasebot; }; then
    restore_units "$prev_unit" "$prev_triage"
    die "could not install or enable the unit; the active version is unchanged"
  fi

  ln -sfn "$LIB_DIR/$release" "$LIB_DIR/current"
  if start_healthy && restart_brokers "$brokers"; then
    rm -f "$prev_unit" "$prev_triage"
    echo "$sha" > "$LIB_DIR/current-commit"
    echo "$release" > "$LIB_DIR/current-release"
    echo "releasebot $release is running${brokers:+ (triage broker restarted)}"
    return 0
  fi

  if [ -z "$prev" ]; then
    # First deploy: nothing to fall back to. Stop the bot and drop `current`, so a later deploy
    # never rolls back to this release; its directory stays for diagnosis.
    systemctl disable --now releasebot || true
    systemctl stop 'releasebot-triage@*.service' || true
    rm -f "$LIB_DIR/current" "$LIB_DIR/current-commit" "$LIB_DIR/current-release"
    restore_units "$prev_unit" "$prev_triage"
    die "releasebot $release did not stay up; the unit is stopped and disabled (see journalctl -u releasebot)"
  fi

  restore_units "$prev_unit" "$prev_triage"
  [ "$prev" = "$LIB_DIR/$release" ] ||
    ln -sfn "$prev" "$LIB_DIR/current"
  if start_healthy && restart_brokers "$brokers"; then
    die "releasebot $release (or its triage broker) did not stay up; rolled back to $(basename "$prev"), which is running"
  fi
  die "releasebot $release did not stay up and $(basename "$prev") is not fully running either" \
    "(see journalctl -u releasebot -u 'releasebot-triage@*')"
}

# owner_mode prints "<uid> <octal mode>" (GNU or BSD stat).
owner_mode() {
  stat -c '%u %a' "$1" 2>/dev/null || stat -f '%u %Lp' "$1"
}

# active_brokers lists the triage broker instances running now.
active_brokers() {
  systemctl list-units --state=active --plain --no-legend 'releasebot-triage@*.service' 2>/dev/null |
    awk '{print $1}' || true
}

# restart_brokers moves the brokers that were running to the binary `current` points at (bot and
# broker run the same release) and fails if one does not stay up, e.g. a skill that no longer matches.
restart_brokers() {
  local b
  [ -n "$1" ] || return 0
  for b in $1; do systemctl restart "$b" || return 1; done
  sleep "$SETTLE_SECONDS"
  for b in $1; do systemctl is-active --quiet "$b" || { echo "triage broker $b did not stay up" >&2; return 1; }; done
}

# save_unit copies an installed unit aside (empty when none) and prints the copy's path.
save_unit() {
  local saved
  saved="$(mktemp)"
  if [ -f "$1" ]; then cp -p "$1" "$saved"; else : > "$saved"; fi
  echo "$saved"
}

# restore_units puts the previously installed units back (or removes first-time installs).
restore_units() {
  local saved dest pair
  for pair in "$1:$UNIT" "$2:$TRIAGE_UNIT"; do
    saved="${pair%%:*}" dest="${pair#*:}"
    if [ -s "$saved" ]; then cp -p "$saved" "$dest"; else rm -f "$dest"; fi
    rm -f "$saved"
  done
  systemctl daemon-reload || true
}

case "${1:-}" in
  prepare) shift; prepare "${1:-}" ;;
  deploy) shift; [ $# -eq 3 ] || die "usage: $0 deploy <binary> <matrix.yaml> <commit-sha>"; deploy "$@" ;;
  *) echo "usage: $0 prepare [triage-user] | deploy <binary> <matrix.yaml> <commit-sha>" >&2; exit 64 ;;
esac
