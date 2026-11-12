-- The variant objects stay in S3 under media/<id>/; nothing refers to them after this.
DROP INDEX IF EXISTS media_variants_pending_idx;

ALTER TABLE media
    DROP CONSTRAINT IF EXISTS media_crop_check,
    DROP COLUMN IF EXISTS variants_claimed_at,
    DROP COLUMN IF EXISTS variants_error,
    DROP COLUMN IF EXISTS variants_attempts,
    DROP COLUMN IF EXISTS variants_status,
    DROP COLUMN IF EXISTS variants,
    DROP COLUMN IF EXISTS crop_h,
    DROP COLUMN IF EXISTS crop_w,
    DROP COLUMN IF EXISTS crop_y,
    DROP COLUMN IF EXISTS crop_x,
    DROP COLUMN IF EXISTS kind;
