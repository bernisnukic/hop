CREATE TABLE links (
    code       text        PRIMARY KEY,
    url        text        NOT NULL,
    hits       bigint      NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now()
);
