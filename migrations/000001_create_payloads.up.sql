CREATE TABLE IF NOT EXISTS payloads (
    payload_id        UUID      NOT NULL,
    kind              TEXT      NOT NULL CHECK (kind IN ('text', 'file')),
    status            TEXT      NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    body_text         TEXT      NOT NULL DEFAULT '',
    original_filename TEXT      NOT NULL DEFAULT '',
    content_type      TEXT      NOT NULL DEFAULT '',
    object_key        TEXT      NOT NULL DEFAULT '',
    size_bytes        BIGINT    NOT NULL DEFAULT 0,
    result_text       TEXT      NOT NULL DEFAULT '',
    error_text        TEXT      NOT NULL DEFAULT '',
    date_created      TIMESTAMP NOT NULL,
    date_updated      TIMESTAMP NOT NULL,

    PRIMARY KEY (payload_id)
);

CREATE INDEX IF NOT EXISTS idx_payloads_status ON payloads (status);
CREATE INDEX IF NOT EXISTS idx_payloads_date_created ON payloads (date_created);
