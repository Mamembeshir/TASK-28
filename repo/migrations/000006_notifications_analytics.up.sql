-- notifications
CREATE TABLE IF NOT EXISTS notifications (
    id          UUID        PRIMARY KEY,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type  VARCHAR(64) NOT NULL,
    title       VARCHAR(300) NOT NULL,
    body        TEXT        NOT NULL,
    resource_id UUID,
    is_read     BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP   NOT NULL DEFAULT NOW(),
    read_at     TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread ON notifications(user_id, is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_user_created ON notifications(user_id, created_at DESC);

-- notification_subscriptions (per-user per-event-type opt-in/out)
CREATE TABLE IF NOT EXISTS notification_subscriptions (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type  VARCHAR(64) NOT NULL,
    enabled     BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at  TIMESTAMP   NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, event_type)
);

-- notification_retry_queue
CREATE TABLE IF NOT EXISTS notification_retry_queue (
    id            UUID        PRIMARY KEY,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_type    VARCHAR(64) NOT NULL,
    title         VARCHAR(300) NOT NULL,
    body          TEXT        NOT NULL,
    resource_id   UUID,
    attempts      INT         NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMP   NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    created_at    TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP   NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notif_retry_pending ON notification_retry_queue(status, next_retry_at)
    WHERE status = 'PENDING';

-- analytics_summary (pre-computed metrics, refreshed by scheduled job)
CREATE TABLE IF NOT EXISTS analytics_summary (
    id           UUID               PRIMARY KEY,
    metric_type  VARCHAR(64)        NOT NULL,
    metric_key   VARCHAR(256)       NOT NULL,
    metric_value DOUBLE PRECISION   NOT NULL DEFAULT 0,
    metric_label VARCHAR(300)       NOT NULL DEFAULT '',
    computed_at  TIMESTAMP          NOT NULL DEFAULT NOW(),
    period_start TIMESTAMP          NOT NULL DEFAULT NOW(),
    period_end   TIMESTAMP          NOT NULL DEFAULT NOW(),
    UNIQUE (metric_type, metric_key)
);
CREATE INDEX IF NOT EXISTS idx_analytics_summary_type ON analytics_summary(metric_type);

-- scheduled_reports
CREATE TABLE IF NOT EXISTS scheduled_reports (
    id           UUID        PRIMARY KEY,
    report_type  VARCHAR(64) NOT NULL,
    parameters   JSONB       NOT NULL DEFAULT '{}',
    file_path    TEXT        NOT NULL DEFAULT '',
    status       VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    generated_at TIMESTAMP,
    requested_by UUID        NOT NULL REFERENCES users(id),
    created_at   TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP   NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_scheduled_reports_requested_by ON scheduled_reports(requested_by);
