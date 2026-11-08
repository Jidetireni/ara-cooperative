-- +goose Up
ALTER TABLE campaigns
    ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'CLOSED', 'CANCELLED')),
    ADD COLUMN status_reason TEXT,
    ADD COLUMN status_changed_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN status_changed_at TIMESTAMPTZ,
    ADD COLUMN resolution TEXT,
    ADD COLUMN resolved_by UUID REFERENCES users(id) ON DELETE RESTRICT,
    ADD COLUMN resolved_at TIMESTAMPTZ,
    ADD COLUMN updated_at TIMESTAMPTZ,
    ADD CONSTRAINT campaigns_status_record CHECK (
        (status = 'ACTIVE' AND status_reason IS NULL AND status_changed_by IS NULL AND status_changed_at IS NULL)
        OR (status <> 'ACTIVE' AND status_reason IS NOT NULL AND btrim(status_reason) <> '' AND status_changed_by IS NOT NULL AND status_changed_at IS NOT NULL)
    ),
    ADD CONSTRAINT campaigns_resolution_record CHECK (
        (resolution IS NULL AND resolved_by IS NULL AND resolved_at IS NULL)
        OR (status = 'CANCELLED' AND resolution IS NOT NULL AND btrim(resolution) <> '' AND resolved_by IS NOT NULL AND resolved_at IS NOT NULL)
    );

-- +goose Down
ALTER TABLE campaigns
    DROP CONSTRAINT campaigns_status_record,
    DROP CONSTRAINT campaigns_resolution_record,
    DROP COLUMN status,
    DROP COLUMN status_reason,
    DROP COLUMN status_changed_by,
    DROP COLUMN status_changed_at,
    DROP COLUMN resolution,
    DROP COLUMN resolved_by,
    DROP COLUMN resolved_at,
    DROP COLUMN updated_at;
