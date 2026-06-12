#!/bin/bash
# ============================================================================
# LICENSE: MongoDB Community Server is SSPL-1.0 — *** NOT OSI-APPROVED ***.
# The OSI explicitly declined the SSPL; it is NOT open source. Caution tier.
# Clean, truly-open alternative: QuenchWorks' FerretDB + standalone documentdb
# (MongoDB-wire compatible, Apache-2.0/PostgreSQL-licensed) — prefer them.
# ============================================================================
#
# MongoDB entrypoint for a read-only rootfs. Ensures a writable dbpath + log dir,
# does a first-boot root-user auth bootstrap when MONGO_INITDB_ROOT_USERNAME /
# MONGO_INITDB_ROOT_PASSWORD are set (mirroring the official mongo image), then
# exec's mongod bound to all interfaces on 27017.
set -euo pipefail

DBPATH="${MONGO_DATA_DIR:-/data/db}"
LOGDIR="${MONGO_LOG_DIR:-/data/log}"
PORT="${MONGO_PORT:-27017}"

# mongosh ships a companion cryptography .so next to the binary; expose it.
export LD_LIBRARY_PATH="/usr/lib/mongosh${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"

# read-only rootfs: mongosh writes config/history under $HOME (default would be a
# non-writable path). Point HOME at a writable location so the bootstrap client
# (and any exec'd probe) can run.
export HOME="${MONGO_LOG_DIR:-/data/log}"

# read-only rootfs: every writable area must live on a mount. mongod also creates
# a unix domain socket (default /tmp/mongodb-<port>.sock) which would hit the
# read-only rootfs — relocate it under the writable log dir via
# --unixSocketPrefix.
mkdir -p "$DBPATH" "$LOGDIR"
SOCKDIR="$LOGDIR"

ROOT_USER="${MONGO_INITDB_ROOT_USERNAME:-}"
ROOT_PASS="${MONGO_INITDB_ROOT_PASSWORD:-}"
INIT_DB="${MONGO_INITDB_DATABASE:-}"

# Decide whether auth is enabled: only if a root user is configured AND we have
# (or will have) created it. We mark a configured datadir with a sentinel.
AUTH_SENTINEL="$DBPATH/.quench-auth-enabled"

# detect "fresh" datadir (no existing WiredTiger storage)
is_fresh() { [ ! -f "$DBPATH/WiredTiger" ]; }

bootstrap_root_user() {
  echo "[entrypoint] first boot: bootstrapping root user '$ROOT_USER' in admin db"
  # start mongod locally (localhost only, no auth) to create the root user
  mongod --dbpath "$DBPATH" --bind_ip 127.0.0.1 --port "$PORT" \
    --unixSocketPrefix "$SOCKDIR" \
    --logpath "$LOGDIR/init-mongod.log" --fork

  # wait for it to accept connections
  for i in $(seq 1 60); do
    if mongosh --quiet --host 127.0.0.1 --port "$PORT" \
        --eval 'db.runCommand({ping:1}).ok' 2>/dev/null | grep -q 1; then
      break
    fi
    [ "$i" = 60 ] && { echo "[entrypoint] FATAL: local mongod did not come up"; cat "$LOGDIR/init-mongod.log" >&2 || true; exit 1; }
    sleep 1
  done

  # create the root user (admin db). Pass creds via env to avoid CLI/log leakage.
  MONGO_NEWUSER="$ROOT_USER" MONGO_NEWPASS="$ROOT_PASS" \
  mongosh --quiet --host 127.0.0.1 --port "$PORT" admin --eval '
    db.createUser({
      user: process.env.MONGO_NEWUSER,
      pwd: process.env.MONGO_NEWPASS,
      roles: [ { role: "root", db: "admin" } ]
    });
  '

  # optional: create an initial (empty) database by touching a collection
  if [ -n "$INIT_DB" ]; then
    echo "[entrypoint] creating initial database '$INIT_DB'"
    mongosh --quiet --host 127.0.0.1 --port "$PORT" "$INIT_DB" \
      --eval 'db.createCollection("quench_init");' || true
  fi

  # stop the bootstrap instance cleanly (db.shutdownServer drops the connection)
  mongosh --quiet --host 127.0.0.1 --port "$PORT" admin \
    --eval 'db.shutdownServer({force:true})' >/dev/null 2>&1 || true
  # wait until it no longer answers ping, then ensure it's gone
  for i in $(seq 1 30); do
    if ! mongosh --quiet --host 127.0.0.1 --port "$PORT" \
        --eval 'db.runCommand({ping:1}).ok' >/dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  pkill -x mongod 2>/dev/null || true
  sleep 1
  touch "$AUTH_SENTINEL"
  echo "[entrypoint] root user bootstrap complete; auth will be enforced"
}

if [ -n "$ROOT_USER" ] && [ -n "$ROOT_PASS" ]; then
  if is_fresh && [ ! -f "$AUTH_SENTINEL" ]; then
    bootstrap_root_user
  fi
  echo "[entrypoint] starting mongod WITH --auth on 0.0.0.0:$PORT"
  exec mongod --dbpath "$DBPATH" --bind_ip_all --port "$PORT" \
    --unixSocketPrefix "$SOCKDIR" \
    --logpath "$LOGDIR/mongod.log" --logappend --auth
else
  # No root creds configured: run open (NetworkPolicy is the boundary). Noted.
  echo "[entrypoint] NOTE: MONGO_INITDB_ROOT_USERNAME/PASSWORD unset — running WITHOUT auth."
  echo "[entrypoint] NetworkPolicy is the security boundary in this mode."
  exec mongod --dbpath "$DBPATH" --bind_ip_all --port "$PORT" \
    --unixSocketPrefix "$SOCKDIR" \
    --logpath "$LOGDIR/mongod.log" --logappend
fi
