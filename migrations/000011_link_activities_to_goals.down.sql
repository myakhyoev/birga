-- Restores the single goal key from the activity's first goal, matched by the names the up
-- migration matches on; anything else falls back to 'cognitive'. Goals created by the up migration stay.
ALTER TABLE activities ADD COLUMN IF NOT EXISTS goal TEXT NOT NULL DEFAULT 'cognitive';

WITH keys (goal, name_uz, name_ru, name_en) AS (
    VALUES
        ('language',  'Nutq',        'Речь',       'Language'),
        ('motor',     'Harakat',     'Моторика',   'Motor skills'),
        ('cognitive', 'Fikrlash',    'Мышление',   'Thinking'),
        ('social',    'Muloqot',     'Общение',    'Social skills'),
        ('emotional', 'Hissiyotlar', 'Эмоции',     'Emotions')
)
UPDATE activities a SET goal = k.goal
FROM goals g JOIN keys k ON
    LOWER(g.name_en) IN (LOWER(k.name_en), k.goal) OR
    LOWER(g.name_uz) = LOWER(k.name_uz) OR
    LOWER(g.name_ru) = LOWER(k.name_ru)
WHERE g.id = a.goal_ids[1];

ALTER TABLE activities ALTER COLUMN goal DROP DEFAULT;

DROP INDEX IF EXISTS activities_published_goal_ids_idx;
ALTER TABLE activities DROP COLUMN IF EXISTS goal_ids;

CREATE INDEX IF NOT EXISTS activities_published_goal_idx ON activities (goal) WHERE is_published;
