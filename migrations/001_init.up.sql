CREATE TABLE IF NOT EXISTS chats (
    id         BIGINT PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS links (
    id         BIGSERIAL PRIMARY KEY,
    url        TEXT        NOT NULL UNIQUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS chat_links (
    chat_id    BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    link_id    BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chat_id, link_id)
);

CREATE TABLE IF NOT EXISTS link_tags (
    id      BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    tag     TEXT   NOT NULL,
    UNIQUE (chat_id, link_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_links_url        ON links(url);
CREATE INDEX IF NOT EXISTS idx_chat_links_chat  ON chat_links(chat_id);
CREATE INDEX IF NOT EXISTS idx_chat_links_link  ON chat_links(link_id);
CREATE INDEX IF NOT EXISTS idx_link_tags_chat   ON link_tags(chat_id);
CREATE INDEX IF NOT EXISTS idx_link_tags_link   ON link_tags(link_id);