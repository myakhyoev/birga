#!/usr/bin/env bash
# Downloads every seed photo listed in seeds/photos/sources.tsv whose JPEG is not in the repo yet,
# and stores it as a 1200 px wide JPEG. Runs in the "Fetch seed photos" workflow (the cloud
# sessions cannot reach the image host). See docs/operations.md, "Seed photos".
set -euo pipefail
cd "$(dirname "$0")/../.."
fetched=0
while IFS=$'\t' read -r kind key file url; do
	[[ -z "$kind" || "$kind" == \#* ]] && continue
	dest="seeds/photos/$file"
	[[ -f "$dest" ]] && continue
	mkdir -p "$(dirname "$dest")"
	tmp="$(mktemp)"
	if ! curl -fsSL --retry 3 -o "$tmp" "$url"; then
		echo "::error::could not download the photo for $kind $key"
		rm -f "$tmp"
		exit 1
	fi
	convert "$tmp" -resize '1200x1200>' -strip -interlace JPEG -quality 82 "jpg:$dest"
	rm -f "$tmp"
	echo "fetched $dest ($(du -k "$dest" | cut -f1) KB)"
	fetched=$((fetched + 1))
done <seeds/photos/sources.tsv
echo "$fetched photo(s) fetched"
