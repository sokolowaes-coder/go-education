-- IF NOT EXISTS: таблица могла быть создана до перехода на golang-migrate.
CREATE TABLE IF NOT EXISTS records (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name       TEXT        NOT NULL CHECK (btrim(name) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
