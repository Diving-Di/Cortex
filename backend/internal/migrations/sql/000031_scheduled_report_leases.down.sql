DROP INDEX IF EXISTS ix_scheduled_report_claim;
ALTER TABLE scheduled_report_runs DROP COLUMN IF EXISTS lease_owner;
ALTER TABLE scheduled_report_tasks DROP COLUMN IF EXISTS lease_until, DROP COLUMN IF EXISTS lease_owner;
