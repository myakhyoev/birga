#!/usr/bin/env bash
# Runs on the server after CI uploaded birga_backend.new, migrations/ and seeds/:
# applies migrations, loads the starter activities into an empty database, attaches the starter
# pictures, swaps the binary
# and restarts the API. If the new version fails its health check, the previous one comes back.
set -euo pipefail
cd /opt/birga/backend
set -a
. /etc/birga/backend.env
set +a

migrate -path migrations -database "$POSTGRES_URL" up

if [[ "$(psql "$POSTGRES_URL" -tAc 'SELECT count(*) FROM activities')" == "0" ]]; then
	psql "$POSTGRES_URL" -q -v ON_ERROR_STOP=1 -f seeds/activities.sql
	echo "starter activities loaded"
fi

chmod 755 birga_backend.new

# Starter pictures: activities, articles and videos without a picture get theirs from seeds/photos.
# A failure here does not stop the release; the next deploy tries again.
./birga_backend.new seed-photos seeds/photos || echo "seed photos failed; see the log above" >&2
[[ -f birga_backend ]] && cp -p birga_backend birga_backend.prev
mv birga_backend.new birga_backend
systemctl restart birga-backend

for _ in $(seq 1 30); do
	if curl -fsS http://127.0.0.1:8080/health >/dev/null; then
		echo "API healthy"
		exit 0
	fi
	sleep 1
done

echo "API failed its health check; recent log:" >&2
journalctl -u birga-backend -n 30 --no-pager -o cat | grep -v -i -E 'password|secret|code' >&2 || true
if [[ -f birga_backend.prev ]]; then
	mv birga_backend.prev birga_backend
	systemctl restart birga-backend
	echo "rolled back to the previous version" >&2
fi
exit 1
