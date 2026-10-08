-- Activities now point at rows of the goals table (the goals a parent picks at sign-up) instead of
-- the five fixed goal keys. An activity can serve several goals.
ALTER TABLE activities ADD COLUMN IF NOT EXISTS goal_ids UUID[] NOT NULL DEFAULT '{}';

-- Map each old key to a goal: reuse an active goal that already has one of the names, otherwise
-- create it. Goals are created only for keys that some activity uses. Like users.goal_ids, the
-- array cannot carry a foreign key; deleting a goal removes its id here too (see goalRepo.Delete).
WITH keys (goal, name_uz, name_ru, name_en) AS (
    VALUES
        ('language',  'Nutq',        'Речь',       'Language'),
        ('motor',     'Harakat',     'Моторика',   'Motor skills'),
        ('cognitive', 'Fikrlash',    'Мышление',   'Thinking'),
        ('social',    'Muloqot',     'Общение',    'Social skills'),
        ('emotional', 'Hissiyotlar', 'Эмоции',     'Emotions')
), existing AS (
    SELECT DISTINCT ON (k.goal) k.goal, g.id
    FROM keys k
    JOIN goals g ON g.deleted_at IS NULL AND (
        LOWER(g.name_en) IN (LOWER(k.name_en), k.goal) OR
        LOWER(g.name_uz) = LOWER(k.name_uz) OR
        LOWER(g.name_ru) = LOWER(k.name_ru))
    ORDER BY k.goal, g.created_at, g.id
), inserted AS (
    INSERT INTO goals (name_uz, name_ru, name_en)
    SELECT k.name_uz, k.name_ru, k.name_en
    FROM keys k
    WHERE k.goal NOT IN (SELECT goal FROM existing)
      AND EXISTS (SELECT 1 FROM activities a WHERE a.goal = k.goal)
    RETURNING id, name_en
), mapping AS (
    SELECT goal, id FROM existing
    UNION ALL
    SELECT k.goal, i.id FROM inserted i JOIN keys k ON k.name_en = i.name_en
)
UPDATE activities a SET goal_ids = ARRAY[m.id]
FROM mapping m
WHERE m.goal = a.goal;

DROP INDEX IF EXISTS activities_published_goal_idx;
ALTER TABLE activities DROP COLUMN IF EXISTS goal;

CREATE INDEX IF NOT EXISTS activities_published_goal_ids_idx ON activities USING GIN (goal_ids)
    WHERE is_published AND deleted_at IS NULL;
