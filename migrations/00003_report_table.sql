-- +goose Up
CREATE TABLE reports(
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    report_type VARCHAR(50) NOT NULL,
    output_file_path VARCHAR,
    download_url VARCHAR,
    download_url_expires_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    failed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    PRIMARY KEY (user_id, id)
);

CREATE INDEX idx_reports_user_id ON reports(user_id);

CREATE INDEX idx_reports_report_type ON reports(report_type);

-- +goose Down
DROP INDEX idx_reports_user_id;

DROP INDEX idx_reports_report_type;

DROP TABLE reports;
