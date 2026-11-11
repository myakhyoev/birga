-- Pictures for activities (the photo on the activity card and page) and cover pictures for video
-- posts. Both are uploaded media rows; deleting the media row clears the picture. A NULL photo
-- means the app shows its own placeholder.
ALTER TABLE activities
    ADD COLUMN IF NOT EXISTS photo_id UUID CONSTRAINT activities_photo_id_fkey REFERENCES media (id) ON DELETE SET NULL;

ALTER TABLE posts
    ADD COLUMN IF NOT EXISTS photo_id UUID CONSTRAINT posts_photo_id_fkey REFERENCES media (id) ON DELETE SET NULL;
