-- Resized copies (variants) of uploaded photos. The API keeps the original and a worker inside the
-- service makes the sizes of the photo's kind: profile (square) or content (2:1 banner for
-- activities, articles and video covers). See docs/architecture.md, "Photo variants".
ALTER TABLE media
    ADD COLUMN IF NOT EXISTS kind TEXT CONSTRAINT media_kind_check CHECK (kind IN ('profile', 'content')),
    ADD COLUMN IF NOT EXISTS crop_x DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS crop_y DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS crop_w DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS crop_h DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS variants JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS variants_status TEXT NOT NULL DEFAULT 'none'
        CONSTRAINT media_variants_status_check CHECK (variants_status IN ('none', 'pending', 'done', 'failed')),
    ADD COLUMN IF NOT EXISTS variants_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS variants_error TEXT,
    ADD COLUMN IF NOT EXISTS variants_claimed_at TIMESTAMPTZ,
    ADD CONSTRAINT media_crop_check CHECK (
        (crop_x IS NULL AND crop_y IS NULL AND crop_w IS NULL AND crop_h IS NULL)
        OR (crop_x >= 0 AND crop_y >= 0 AND crop_w > 0 AND crop_h > 0
            AND crop_x + crop_w <= 1.000001 AND crop_y + crop_h <= 1.000001));

-- The worker picks pending rows oldest first.
CREATE INDEX IF NOT EXISTS media_variants_pending_idx ON media (created_at) WHERE variants_status = 'pending';

-- Backfill: give the photos already in use a kind from where they are used, and queue them.
-- Content wins if one photo is used both ways (never the case today).
UPDATE media SET kind = 'profile'
WHERE kind IS NULL AND content_type LIKE 'image/%'
  AND (id IN (SELECT photo_id FROM users WHERE photo_id IS NOT NULL)
    OR id IN (SELECT photo_id FROM children WHERE photo_id IS NOT NULL));

UPDATE media SET kind = 'content'
WHERE content_type LIKE 'image/%'
  AND (id IN (SELECT photo_id FROM activities WHERE photo_id IS NOT NULL)
    OR id IN (SELECT photo_id FROM articles WHERE photo_id IS NOT NULL)
    OR id IN (SELECT photo_id FROM posts WHERE photo_id IS NOT NULL));

UPDATE media SET variants_status = 'pending' WHERE kind IS NOT NULL AND variants_status = 'none';
