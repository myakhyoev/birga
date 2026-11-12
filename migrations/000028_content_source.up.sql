-- Where a piece of content comes from: test (sample content made for testing), admin (written in
-- the dashboard), harvard (taken from Harvard material) or ai (generated with AI).
-- Every activity, article and video stored so far is AI-made test content, so existing rows
-- become 'test'; new rows default to 'admin'.
ALTER TABLE activities
    ADD COLUMN source TEXT NOT NULL DEFAULT 'test'
        CONSTRAINT activities_source_chk CHECK (source IN ('test', 'admin', 'harvard', 'ai'));
ALTER TABLE activities ALTER COLUMN source SET DEFAULT 'admin';

ALTER TABLE articles
    ADD COLUMN source TEXT NOT NULL DEFAULT 'test'
        CONSTRAINT articles_source_chk CHECK (source IN ('test', 'admin', 'harvard', 'ai'));
ALTER TABLE articles ALTER COLUMN source SET DEFAULT 'admin';

ALTER TABLE posts
    ADD COLUMN source TEXT NOT NULL DEFAULT 'test'
        CONSTRAINT posts_source_chk CHECK (source IN ('test', 'admin', 'harvard', 'ai'));
ALTER TABLE posts ALTER COLUMN source SET DEFAULT 'admin';
