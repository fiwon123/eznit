CREATE TABLE files_history (
    id BIGSERIAL PRIMARY KEY,
    file_id BIGSERIAL NOT NULL,
    path TEXT,
    version INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
