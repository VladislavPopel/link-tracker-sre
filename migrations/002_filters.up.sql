CREATE TABLE IF NOT EXISTS link_filters (
    id      BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    link_id BIGINT NOT NULL REFERENCES links(id) ON DELETE CASCADE,
    filter  TEXT   NOT NULL,
    UNIQUE (chat_id, link_id, filter)
);

CREATE INDEX IF NOT EXISTS idx_link_filters_chat_link ON link_filters(chat_id, link_id);