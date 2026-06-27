CREATE TABLE IF NOT EXISTS jobs (
    job_id       UUID        NOT NULL,
    payload_id   UUID        NOT NULL REFERENCES payloads (payload_id) ON DELETE CASCADE,
    status       TEXT        NOT NULL DEFAULT 'queued'
                             CHECK (status IN ('queued', 'running', 'done', 'failed')),
    attempts     INT         NOT NULL DEFAULT 0,
    max_attempts INT         NOT NULL DEFAULT 5,
    last_error   TEXT        NOT NULL DEFAULT '',
    run_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at    TIMESTAMPTZ NULL,
    locked_until TIMESTAMPTZ NULL,
    date_created TIMESTAMPTZ NOT NULL DEFAULT now(),
    date_updated TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (job_id)
);

-- Partial index so the SKIP LOCKED dequeue only scans claimable rows.
CREATE INDEX IF NOT EXISTS idx_jobs_claimable
    ON jobs (run_after)
    WHERE status IN ('queued', 'running');
