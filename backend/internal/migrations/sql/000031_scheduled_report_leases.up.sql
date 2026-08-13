ALTER TABLE scheduled_report_tasks
    ADD COLUMN lease_owner uuid,
    ADD COLUMN lease_until timestamptz;

ALTER TABLE scheduled_report_runs
    ADD COLUMN lease_owner uuid;

CREATE INDEX ix_scheduled_report_claim ON scheduled_report_tasks(status, next_run_at, lease_until, id)
    WHERE status='enabled';
