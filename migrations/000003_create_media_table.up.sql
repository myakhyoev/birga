CREATE TABLE IF NOT EXISTS media (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    object_key   TEXT        NOT NULL UNIQUE,
    content_type TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL CHECK (size_bytes > 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A user's photo must be an uploaded media row; deleting the media row clears the photo.
ALTER TABLE users
    ADD CONSTRAINT users_photo_id_fkey FOREIGN KEY (photo_id) REFERENCES media (id) ON DELETE SET NULL;
