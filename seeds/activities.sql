-- Starter goals and activities for local development and demos: five goals, a few activities per
-- goal across ages 2 to 6. Safe to run more than once: a goal is skipped when an active goal already
-- has its English name, an activity when one with the same title_uz exists.
-- The texts are drafts; have the content team review them before using this outside development.
INSERT INTO goals (name_uz, name_ru, name_en)
SELECT g.name_uz, g.name_ru, g.name_en
FROM (VALUES
    ('Nutq',        'Речь',     'Language'),
    ('Harakat',     'Моторика', 'Motor skills'),
    ('Fikrlash',    'Мышление', 'Thinking'),
    ('Muloqot',     'Общение',  'Social skills'),
    ('Hissiyotlar', 'Эмоции',   'Emotions')
) AS g (name_uz, name_ru, name_en)
WHERE NOT EXISTS (SELECT 1 FROM goals x WHERE LOWER(x.name_en) = LOWER(g.name_en) AND x.deleted_at IS NULL);

-- Each activity names its goal by key; keys maps the key to the goal's English name above.
INSERT INTO activities (title_uz, title_ru, description_uz, description_ru, goal_ids, min_age, max_age, duration_minutes, is_published)
SELECT s.title_uz, s.title_ru, s.description_uz, s.description_ru, ARRAY[g.id], s.min_age, s.max_age, s.duration_minutes, TRUE
FROM (VALUES
    ('Kim qanday ovoz chiqaradi?', 'Кто как говорит?',
     'Hayvonlar rasmini yoki o''yinchoqlarini birma-bir ko''rsating va «Mushuk qanday qiladi?» deb so''rang. Bola javob bersa, ovozni birga takrorlang va hayvon nomini aniq ayting.',
     'Показывайте по одной картинке или игрушке животного и спрашивайте: «Как говорит кошка?». Когда ребёнок ответит, повторите звук вместе и чётко назовите животное.',
     'language', 2, 3, 5),
    ('Qopdagi sirli narsa', 'Волшебный мешочек',
     'Qopga 4–5 ta tanish narsa soling. Bola qo''lini tiqib, bittasini ushlab ko''rsin va qanday ekanini aytsin: yumshoqmi, qattiqmi, katta yoki kichik. Keyin chiqarib nomini aytsin.',
     'Положите в мешочек 4–5 знакомых предметов. Ребёнок на ощупь выбирает один и описывает его: мягкий или твёрдый, большой или маленький. Потом достаёт и называет предмет.',
     'language', 3, 6, 10),
    ('Bugun nima qildik?', 'Что мы сегодня делали?',
     'Kechqurun bola bilan kunni eslang: ertalab nima qildik, kim bilan o''ynadik, eng qiziq narsa nima bo''ldi. Har bir javobni bitta qo''shimcha savol bilan davom ettiring.',
     'Вечером вспомните с ребёнком день: что делали утром, с кем играли, что было самым интересным. Каждый ответ продолжайте одним уточняющим вопросом.',
     'language', 4, 6, 10),
    ('Yostiq yo''lagi', 'Дорожка из подушек',
     'Yerga yostiqlarni qator qilib qo''ying. Bola ularning ustidan yiqilmasdan yursin, keyin sakrab o''tsin. Qo''lini ushlab turing va har safar yangi usulni taklif qiling.',
     'Разложите подушки в ряд на полу. Ребёнок проходит по ним, не падая, потом перепрыгивает. Держите за руку и каждый раз предлагайте новый способ.',
     'motor', 2, 4, 10),
    ('Qog''oz yirtib rasm', 'Рваная аппликация',
     'Rangli qog''ozni bola mayda bo''laklarga yirtsin va ularni yelim bilan oddiy shakl (quyosh, daraxt) ichiga yopishtirsin. Barmoqlar bilan ishlashga e''tibor bering.',
     'Ребёнок рвёт цветную бумагу на мелкие кусочки и наклеивает их внутрь простого контура (солнце, дерево). Обращайте внимание на работу пальчиков.',
     'motor', 3, 6, 15),
    ('Ip o''tkazish', 'Нанизывание бусин',
     'Yo''g''on ipga katta munchoq yoki kesilgan makaronlarni o''tkazing. Avval o''zingiz ko''rsating, keyin bola rang navbatini o''zi tanlasin.',
     'Нанизывайте на толстую нитку крупные бусины или нарезанные макароны. Сначала покажите сами, затем пусть ребёнок сам выберет порядок цветов.',
     'motor', 4, 6, 10),
    ('Rangli toshlar', 'Цветные камни',
     'Toshlar, tugmalar yoki qopqoqlarni rangi bo''yicha idishlarga ajrating. Bola tayyor bo''lsa, kattaligi bo''yicha ham saralab ko''ring.',
     'Разложите камешки, пуговицы или крышки по мисочкам по цвету. Если ребёнок готов, попробуйте рассортировать их и по размеру.',
     'cognitive', 2, 4, 10),
    ('Nima yo''qoldi?', 'Чего не стало?',
     'Stolga 4–5 ta narsa qo''ying. Bola ko''zini yumganda bittasini olib qo''ying. Ko''zini ochgach, nima yo''qolganini topsin. Keyin rollarni almashtiring.',
     'Положите на стол 4–5 предметов. Пока ребёнок закрыл глаза, уберите один. Открыв глаза, он угадывает, чего не стало. Потом поменяйтесь ролями.',
     'cognitive', 3, 6, 10),
    ('Navbat bilan minora', 'Башня по очереди',
     'Kubiklardan navbat bilan minora quring: bir kubik sizdan, bir kubik boladan. Har safar «Endi sening navbating» deb ayting. Minora qulasa, birga kuling va qaytadan boshlang.',
     'Стройте башню из кубиков по очереди: один кубик вы, один ребёнок. Каждый раз говорите «Теперь твоя очередь». Если башня упала, посмейтесь вместе и начните заново.',
     'social', 2, 4, 5),
    ('Do''konchilik o''yini', 'Игра в магазин',
     'Uydagi narsalardan kichik do''kon tuzing. Bola sotuvchi bo''lsin: salom bersin, nima kerakligini so''rasin va «rahmat» desin. Keyin siz sotuvchi bo''ling.',
     'Устройте маленький магазин из домашних вещей. Ребёнок — продавец: здоровается, спрашивает, что нужно, и говорит «спасибо». Потом продавцом становитесь вы.',
     'social', 4, 6, 15),
    ('Tuyg''ular detektivi', 'Детектив эмоций',
     'Ko''zguga qarab navbat bilan quvonch, xafagarchilik, jahl va hayratni yuz ifodasi bilan ko''rsating. Bola har birini nomlasin. So''ng u qachon shunday his qilganini so''rang va javobini tuzatmasdan qabul qiling.',
     'Глядя в зеркало, по очереди изобразите радость, грусть, злость и удивление. Ребёнок называет каждую эмоцию. Затем спросите, когда он сам так себя чувствовал, и примите ответ, не поправляя его.',
     'emotional', 3, 6, 5),
    ('Sharni puflaymiz', 'Надуваем шарик',
     'Bola bilan birga «sharni puflang»: burundan chuqur nafas oling, qorinni shishiring, keyin og''izdan sekin chiqaring. Jahli chiqqan yoki xafa bo''lgan paytlarda shu mashqni eslating.',
     'Вместе с ребёнком «надувайте шарик»: глубокий вдох носом, животик надувается, затем медленный выдох ртом. Напоминайте об этом упражнении, когда ребёнок злится или расстроен.',
     'emotional', 2, 5, 5)
) AS s (title_uz, title_ru, description_uz, description_ru, goal, min_age, max_age, duration_minutes)
JOIN (VALUES
    ('language', 'Language'), ('motor', 'Motor skills'), ('cognitive', 'Thinking'),
    ('social', 'Social skills'), ('emotional', 'Emotions')
) AS keys (goal, name_en) ON keys.goal = s.goal
JOIN goals g ON LOWER(g.name_en) = LOWER(keys.name_en) AND g.deleted_at IS NULL
WHERE NOT EXISTS (SELECT 1 FROM activities a WHERE a.title_uz = s.title_uz AND a.deleted_at IS NULL);
