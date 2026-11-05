#!/bin/sh
# Release the working tree to this machine: verify, install the binaries,
# and restart the launchd-managed `evie serve` so the scheduler and the web
# UI run the code you just built. One step, so a rebuilt binary can never
# again sit idle while the old one keeps running the daily job.
#
#   scripts/release.sh                 verify, back up, install, restart
#   scripts/release.sh --skip-verify   back up, install and restart only
#
# Before installing, the database and the binaries about to be replaced are
# saved to ~/.evie/backups/release-<UTC stamp>/, so a bad release (or a
# one-way migration) can be undone. The last $keep release backups are kept;
# other folders in ~/.evie/backups are never touched. To roll back, stop
# serve, copy the saved binaries back over the installed ones and evie.db
# over ~/.evie/evie.db (removing evie.db-wal and evie.db-shm), then start it.
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

# Back up before installing. The database stays open in WAL mode while serve
# runs, so it is copied with SQLite's online backup rather than cp, and the
# copy is checked before the release goes ahead.
keep=5
bin_dir="$(go env GOBIN)"
[ -n "$bin_dir" ] || bin_dir="$(go env GOPATH)/bin"
backups="$HOME/.evie/backups"
backup="$backups/release-$(date -u +%Y%m%dT%H%M%SZ)"
old_umask="$(umask)"
umask 077
mkdir -p "$backup"
db="$HOME/.evie/evie.db"
if [ -f "$db" ]; then
	sqlite3 "$db" ".timeout 10000" ".backup '$backup/evie.db'"
	# A single self-contained file: no -wal or -shm beside the copy.
	sqlite3 "$backup/evie.db" "PRAGMA journal_mode=DELETE;" >/dev/null
	rm -f "$backup/evie.db-shm" "$backup/evie.db-wal"
	check="$(sqlite3 "$backup/evie.db" "PRAGMA quick_check;")"
	if [ "$check" != "ok" ]; then
		echo "database backup failed its integrity check: $check" >&2
		exit 1
	fi
else
	echo "no database at $db; nothing to back up"
fi
for pkg in $packages; do
	bin="$bin_dir/$(basename "$pkg")"
	[ -f "$bin" ] && cp -p "$bin" "$backup/"
done
umask "$old_umask"
echo "backed up to $backup"
# Prune older release backups; the stamp sorts by time.
# shellcheck disable=SC2012
ls -1d "$backups"/release-* 2>/dev/null | sort -r | tail -n +$((keep + 1)) | while read -r old; do
	rm -rf "$old"
done

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
