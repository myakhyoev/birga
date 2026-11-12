#!/usr/bin/env bash
# Redacted report for the Server diagnostics workflow (public logs: no secret values).
# Env: GREP (log filter), SINCE (journalctl --since).
set -uo pipefail
mask() {
	sed -E 's/[0-9]{1,3}(\.[0-9]{1,3}){3}/<ip>/g; s/[0-9a-f]{1,4}(:[0-9a-f]{0,4}){3,7}/<ip>/Ig;
		s/; id [0-9a-f]+\)/; id <device>)/g; s/[0-9]{9,}/<digits>/g;
		s/(Bearer|token|password|secret)[^ ,"]*[ :="]+[^ ,"]+/\1 <hidden>/Ig'
}

echo "== services"
systemctl is-active birga-backend postgresql redis-server nginx loki alloy grafana-server 2>&1 | paste -sd' '

echo "== backend settings (set or empty, values hidden)"
for k in ENVIRONMENT S3_BUCKET S3_REGION S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY S3_ENDPOINT S3_PUBLIC_BASE_URL MEDIA_MAX_SIZE SMS_PROVIDER; do
	v="$(grep -E "^${k}=" /etc/birga/backend.env | head -1 | cut -d= -f2-)"
	case "$k" in
	ENVIRONMENT | S3_BUCKET | S3_REGION | MEDIA_MAX_SIZE | SMS_PROVIDER) echo "$k=${v:-<empty>}" ;;
	*) [[ -n "$v" ]] && echo "$k=<set>" || echo "$k=<empty>" ;;
	esac
done

echo "== nginx client_max_body_size"
grep -Rh client_max_body_size /etc/nginx/sites-enabled/ | sort | uniq -c

echo "== API log lines matching '$GREP' since $SINCE (last 60)"
journalctl -u birga-backend --since "$SINCE" -o cat --no-pager | grep -i -- "$GREP" | tail -60 | mask

echo "== API errors since $SINCE (last 30)"
journalctl -u birga-backend --since "$SINCE" -o cat --no-pager | grep -E '"level":"(error|warn)"' | tail -30 | mask

echo "== nginx requests matching '$GREP' (last 40; IPs masked)"
grep -ih -- "$GREP" /var/log/nginx/access.log /var/log/nginx/access.log.1 2>/dev/null | tail -40 |
	mask

echo "== nginx errors (last 20)"
tail -20 /var/log/nginx/error.log 2>/dev/null | mask

echo "== activities with Uzbek text but no English (id, title_uz, empty English fields)"
set -a
. /etc/birga/backend.env
set +a
psql "$POSTGRES_URL" -tA -F ' | ' -c "
SELECT id, title_uz, concat_ws(',',
	CASE WHEN title_en = '' AND title_uz <> '' THEN 'title' END,
	CASE WHEN description_en = '' AND description_uz <> '' THEN 'description' END,
	CASE WHEN main_idea_en = '' AND main_idea_uz <> '' THEN 'main_idea' END,
	CASE WHEN easier_en = '' AND easier_uz <> '' THEN 'easier' END,
	CASE WHEN harder_en = '' AND harder_uz <> '' THEN 'harder' END)
FROM activities
WHERE deleted_at IS NULL AND (
	(title_en = '' AND title_uz <> '') OR (description_en = '' AND description_uz <> '') OR
	(main_idea_en = '' AND main_idea_uz <> '') OR (easier_en = '' AND easier_uz <> '') OR
	(harder_en = '' AND harder_uz <> ''))
ORDER BY id" 2>&1 | mask
echo "active activities: $(psql "$POSTGRES_URL" -tAc 'SELECT count(*) FROM activities WHERE deleted_at IS NULL')"

echo "== all live activities (id | title_uz | has photo)"
if psql "$POSTGRES_URL" -tAc "SELECT 1 FROM information_schema.columns WHERE table_name='activities' AND column_name='photo_id'" | grep -q 1; then
	PHOTO="(photo_id IS NOT NULL)::text"
else
	PHOTO="'n/a'"
fi
psql "$POSTGRES_URL" -tA -F ' | ' -c "SELECT id, title_uz, $PHOTO FROM activities WHERE deleted_at IS NULL ORDER BY id" 2>&1 | mask
