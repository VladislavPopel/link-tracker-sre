CREATE TABLE IF NOT EXISTS updates (
    id          BIGSERIAL   PRIMARY KEY,
    chat_id     BIGINT      NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    url         TEXT        NOT NULL,
    description TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Основной запрос фронтенда: "уведомления чата с id больше N"
CREATE INDEX IF NOT EXISTS idx_updates_chat_id_id ON updates(chat_id, id);