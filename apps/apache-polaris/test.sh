#!/usr/bin/env bash
# Smoke test for a built apache-polaris image. Usage: test.sh <image-ref> [version]
# Read-only root, in-memory metastore with bootstrap credentials: Polaris must report ready,
# issue a token, create a FILE catalog, a namespace and a table (Iceberg HadoopFileIO writes
# the table metadata through the stripped hadoop-client-runtime), and list them back.
# The admin tool must start too.
set -euo pipefail
IMAGE="${1:?usage: test.sh <image-ref> [version]}"
WANT="${2:-}"
NAME="apache-polaris-smoke-$$"
trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true; rm -rf "${CONF:-}"' EXIT

user="$(docker inspect "$IMAGE" --format '{{.Config.User}}')"
[ "$user" = "1001" ] || { echo "expected user 1001, got '$user'"; exit 1; }

# Test-only settings in the image config path (/home/nonroot/config): FILE storage is
# insecure by design and refused unless allowed, and the readiness check flags it.
CONF="$(mktemp -d)"; chmod 755 "$CONF"
cat > "$CONF/application.properties" <<'PROPS'
polaris.features."SUPPORTED_CATALOG_STORAGE_TYPES"=["FILE"]
polaris.features."ALLOW_INSECURE_STORAGE_TYPES"=true
polaris.readiness.ignore-severe-issues=true
PROPS
chmod 644 "$CONF/application.properties"
docker run -d --name "$NAME" --read-only --tmpfs /tmp \
  -e POLARIS_BOOTSTRAP_CREDENTIALS=POLARIS,root,s3cr3t \
  -v "$CONF:/home/nonroot/config:ro" \
  -p 127.0.0.1:18181:8181 -p 127.0.0.1:18182:8182 "$IMAGE" >/dev/null
ok=0
for _ in $(seq 1 120); do
  h="$(curl -fsS http://127.0.0.1:18182/q/health/ready 2>/dev/null || true)"
  grep -q '"status": *"UP"' <<<"$h" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "polaris never became ready"; docker logs "$NAME" 2>&1 | tail -40; exit 1; }

B=http://127.0.0.1:18181/api
tok="$(curl -fsS "$B/catalog/v1/oauth/tokens" -d grant_type=client_credentials \
  -d client_id=root -d client_secret=s3cr3t -d scope=PRINCIPAL_ROLE:ALL)"
T="$(sed -n 's/.*"access_token" *: *"\([^"]*\)".*/\1/p' <<<"$tok")"
[ -n "$T" ] || { echo "no token: $tok"; exit 1; }
H=(-H "Authorization: Bearer $T" -H 'Content-Type: application/json')
curl -fsS "${H[@]}" "$B/management/v1/catalogs" -d '{"catalog":{"name":"quench","type":"INTERNAL",
  "properties":{"default-base-location":"file:///tmp/wh"},
  "storageConfigInfo":{"storageType":"FILE","allowedLocations":["file:///tmp/wh"]}}}' >/dev/null
cats="$(curl -fsS "${H[@]}" "$B/management/v1/catalogs")"
grep -q '"name" *: *"quench"' <<<"$cats" || { echo "catalog not listed: $cats"; exit 1; }
curl -fsS "${H[@]}" "$B/catalog/v1/quench/namespaces" -d '{"namespace":["smoke"]}' >/dev/null
ns="$(curl -fsS "${H[@]}" "$B/catalog/v1/quench/namespaces")"
grep -q '"smoke"' <<<"$ns" || { echo "namespace not listed: $ns"; exit 1; }
curl -fsS "${H[@]}" "$B/catalog/v1/quench/namespaces/smoke/tables" -d '{"name":"t1",
  "schema":{"type":"struct","fields":[{"id":1,"name":"id","type":"long","required":true}]}}' >/dev/null
tb="$(curl -fsS "${H[@]}" "$B/catalog/v1/quench/namespaces/smoke/tables")"
grep -q '"name" *: *"t1"' <<<"$tb" || { echo "table not listed: $tb"; exit 1; }
meta="$(docker exec "$NAME" find /tmp/wh -name '*.metadata.json' 2>/dev/null || true)"
[ -n "$meta" ] || { echo "no table metadata written under /tmp/wh"; exit 1; }

logs="$(docker logs "$NAME" 2>&1)"
[ -z "$WANT" ] || grep -q "$WANT" <<<"$logs" || { echo "version $WANT not in the startup log"; exit 1; }
# FILE storage is a severe readiness issue by design (logged as ERROR); fail on class loading only
if grep -E 'ClassNotFound|NoClassDefFound|LinkageError' <<<"$logs"; then echo "polaris hit a class loading error"; exit 1; fi

adm="$(docker run --rm --read-only --tmpfs /tmp --entrypoint /usr/lib/jvm/java-21-openjdk/bin/java \
  "$IMAGE" -jar /usr/lib/polaris/admin/quarkus-run.jar --help 2>&1 | tr -d '\000' || true)"
grep -q bootstrap <<<"$adm" || { echo "admin tool did not start: $adm"; exit 1; }
echo "smoke test passed (polaris ${WANT:-?}, uid $user, catalog, namespace and table created and listed)"
