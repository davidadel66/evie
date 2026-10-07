#!/bin/sh
# Release the working tree to this machine: verify, install the binaries,
# and restart the launchd-managed `evie serve` so the scheduler and the web
# UI run the code you just built. One step, so a rebuilt binary can never
# again sit idle while the old one keeps running the daily job.
#
#   scripts/release.sh                 verify, install, restart
#   scripts/release.sh --skip-verify   install and restart only
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

verify=1
for arg in "$@"; do
	case "$arg" in
	--skip-verify) verify=0 ;;
	*) echo "usage: scripts/release.sh [--skip-verify]" >&2; exit 2 ;;
	esac
done

if [ "$verify" -eq 1 ]; then
	./scripts/verify-change.sh
fi

packages="./cmd/evie"
# cmd/finance is retired after the finance-daily workflow's first clean week.
[ -d cmd/finance ] && packages="$packages ./cmd/finance"
# shellcheck disable=SC2086
go install $packages
echo "installed: $packages"

agent="com.evie.serve"
if [ -f "$HOME/Library/LaunchAgents/$agent.plist" ]; then
	./scripts/install-serve-agent.sh --restart
	addr="${EVIE_ADDR:-127.0.0.1:6687}"
	for _ in 1 2 3 4 5 6 7 8 9 10; do
		if curl -s -o /dev/null "http://$addr/"; then
			echo "evie serve is up on http://$addr"
			exit 0
		fi
		sleep 1
	done
	echo "evie serve did not answer on http://$addr within 10s; see ~/.evie/logs/serve.log" >&2
	exit 1
fi
echo "no $agent LaunchAgent installed; run scripts/install-serve-agent.sh to add one, or restart evie serve by hand"
