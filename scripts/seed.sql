INSERT INTO chats (id)
SELECT generate_series(1, 1000)
ON CONFLICT DO NOTHING;

INSERT INTO links (url)
SELECT 'https://github.com/user/repo-' || generate_series(1, 100000)
ON CONFLICT DO NOTHING;

INSERT INTO chat_links (chat_id, link_id)
SELECT
    chat_id,
    link_id
FROM (
    SELECT
        c.id AS chat_id,
        l.id AS link_id,
        ROW_NUMBER() OVER (PARTITION BY c.id ORDER BY l.id) AS rn
    FROM chats c
    JOIN links l ON l.id BETWEEN (c.id - 1) * 100 + 1 AND c.id * 100
) sub
ON CONFLICT DO NOTHING;