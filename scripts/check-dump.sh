#!/usr/bin/env bash
# A copy of production's database, migrated by this build (#45): restores a
# backup into a throwaway database on TEST_DATABASE_URL's server, notes the
# migration version and every book's posting totals, starts the server on it
# (which migrates it), waits for /readyz, checks the totals did not move,
# and drops the database (KEEP=1 keeps it).
#
#   make db/check-dump dump=rigel-ledger.dump     # pg_dump -Fc, or .sql / .sql.gz
#
# Nothing is written anywhere but that throwaway database; the server runs
# with mail to the log and no rate fetching. Production's secrets (the SMTP
# password, TOTP seeds) are sealed with its APP_ENCRYPTION_KEY: export it
# first if the check should open them; the server starts without.
set -euo pipefail

dump=${1:?usage: scripts/check-dump.sh <dump file>}
[ -r "$dump" ] || { echo "cannot read $dump" >&2; exit 2; }
: "${TEST_DATABASE_URL:?TEST_DATABASE_URL must name a server where databases may be created}"

name="rigel_dumpcheck_$(date +%s)"
base=${TEST_DATABASE_URL%%\?*}
query=""; [[ $TEST_DATABASE_URL == *\?* ]] && query="?${TEST_DATABASE_URL#*\?}"
admin="${base%/*}/postgres${query}"
target="${base%/*}/${name}${query}"
pid=""
cleanup() {
  [ -n "$pid" ] && kill "$pid" 2>/dev/null && wait "$pid" 2>/dev/null || true
  if [ "${KEEP:-}" != 1 ]; then psql "$admin" -qc "DROP DATABASE IF EXISTS $name" >/dev/null; else echo "kept $name"; fi
}
trap cleanup EXIT

psql "$admin" -qc "CREATE DATABASE $name" >/dev/null
case "$dump" in
  *.sql.gz) gunzip -c "$dump" | psql "$target" -q -v ON_ERROR_STOP=1 >/dev/null ;;
  *.sql) psql "$target" -q -v ON_ERROR_STOP=1 -f "$dump" >/dev/null ;;
  *) pg_restore --no-owner --no-acl -d "$target" "$dump" ;;
esac

totals="SELECT t.book_id, count(*), sum(p.base_amount) FROM postings p JOIN transactions t ON t.id = p.transaction_id GROUP BY 1 ORDER BY 1"
before_version=$(psql "$target" -tAc "SELECT version || CASE WHEN dirty THEN ' (dirty)' ELSE '' END FROM schema_migrations")
before=$(psql "$target" -tAc "$totals")
echo "restored $dump: migration $before_version, $(echo "$before" | grep -c . || true) books with postings"

bin=$(mktemp -d)/rigel-ledger
go build -o "$bin" ./cmd/rigel-ledger
port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
log=$(mktemp)
DATABASE_URL="$target" APP_PORT="$port" APP_ENV=development RATES_ENABLED=false MAIL_DRIVER=log \
  ADMIN_USERNAME= ADMIN_INITIAL_PASSWORD= "$bin" >"$log" 2>&1 &
pid=$!
for _ in $(seq 1 60); do
  curl -sf "http://127.0.0.1:$port/readyz" >/dev/null && break
  kill -0 "$pid" 2>/dev/null || { echo "the server stopped:" >&2; tail -20 "$log" >&2; exit 1; }
  sleep 1
done
curl -sf "http://127.0.0.1:$port/readyz" >/dev/null || { echo "not ready after 60 s:" >&2; tail -20 "$log" >&2; exit 1; }

after_version=$(psql "$target" -tAc "SELECT version || CASE WHEN dirty THEN ' (dirty)' ELSE '' END FROM schema_migrations")
after=$(psql "$target" -tAc "$totals")
echo "migrated to $after_version; the server is ready"
if [ "$before" != "$after" ]; then
  echo "POSTING TOTALS CHANGED:" >&2
  diff <(echo "$before") <(echo "$after") >&2 || true
  exit 1
fi
echo "posting totals per book unchanged"
