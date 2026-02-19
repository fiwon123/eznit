CREATE TABLE files (
    id  BIGSERIAL PRIMARY KEY,
    user_id BIGSERIAL NOT NULL,
    name TEXT,
    ext TEXT,
    path TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id) REFERENCES users(id)
);
