#!/bin/sh
# Install (or reinstall) `evie serve` as a per-user launchd LaunchAgent so
# the web UI and the workflow scheduler are up whenever the owner is logged
# in, and restart if they exit (finance-daily, 2026-10-06).
#
#   scripts/install-serve-agent.sh            install from ~/go/bin/evie
#   scripts/install-serve-agent.sh /path/evie install from another binary
#   scripts/install-serve-agent.sh --restart  reload the agent after `go install ./cmd/evie`
#   scripts/install-serve-agent.sh --uninstall
#
# The agent runs the binary at the path recorded in the plist; after
# `go install ./cmd/evie` run this with --restart so the new binary loads.
# A manually started `evie serve` holding the port must be stopped first:
# the agent would otherwise crash-loop on the bind.
set -eu

label="com.evie.serve"
plist_dir="$HOME/Library/LaunchAgents"
plist="$plist_dir/$label.plist"
template="$(cd "$(dirname "$0")" && pwd)/launchd/$label.plist.template"
domain="gui/$(id -u)"

case "${1:-}" in
--uninstall)
	launchctl bootout "$domain/$label" 2>/dev/null || true
	rm -f "$plist"
	echo "removed $label"
	exit 0
	;;
--restart)
	[ -f "$plist" ] || { echo "$plist is not installed; run without --restart first" >&2; exit 1; }
	launchctl kickstart -k "$domain/$label"
	echo "restarted $label"
	exit 0
	;;
"")
	evie_bin="$HOME/go/bin/evie"
	;;
*)
	evie_bin="$1"
	;;
esac

[ -x "$evie_bin" ] || { echo "$evie_bin is not an executable evie binary" >&2; exit 1; }
addr="${EVIE_ADDR:-127.0.0.1:6687}"
port="${addr##*:}"
if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
	holder="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -Fc | sed -n 's/^c//p' | head -1)"
	echo "port $port is already in use by '$holder'; stop it before installing the agent" >&2
	exit 1
fi

mkdir -p "$plist_dir" "$HOME/.evie/logs"
sed -e "s|__EVIE_BIN__|$evie_bin|g" -e "s|__HOME__|$HOME|g" "$template" >"$plist"
launchctl bootout "$domain/$label" 2>/dev/null || true
launchctl bootstrap "$domain" "$plist"
echo "installed $label -> $evie_bin serve (log: $HOME/.evie/logs/serve.log)"
launchctl print "$domain/$label" | grep -E 'state|pid' | head -3
