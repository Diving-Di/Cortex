"""M4 scheduled report tasks and run history."""

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0006_scheduled_reports"
down_revision = "0005_m2_ai_notes"
branch_labels = None
depends_on = None


def _rls(table: str) -> None:
    op.execute(f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY")
    op.execute(f"ALTER TABLE {table} FORCE ROW LEVEL SECURITY")
    op.execute(
        f"CREATE POLICY {table}_tenant_isolation ON {table} "
        "USING (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid) "
        "WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid)"
    )


def upgrade() -> None:
    op.create_table(
        "scheduled_report_tasks",
        sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True),
        sa.Column("tenant_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("created_by", sa.Integer(), nullable=False),
        sa.Column("report_type", sa.String(20), nullable=False),
        sa.Column("hour", sa.Integer(), server_default="20", nullable=False),
        sa.Column("minute", sa.Integer(), server_default="0", nullable=False),
        sa.Column("timezone", sa.String(64), server_default="Asia/Shanghai", nullable=False),
        sa.Column("status", sa.String(20), server_default="enabled", nullable=False),
        sa.Column("next_run_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("last_run_at", sa.DateTime(timezone=True)),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id"], ["tenants.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["created_by"], ["users.id"]),
        sa.CheckConstraint("report_type IN ('daily','weekly','monthly')", name="ck_scheduled_report_type"),
        sa.CheckConstraint("status IN ('enabled','disabled')", name="ck_scheduled_report_status"),
    )
    op.create_index("ix_scheduled_report_tasks_tenant_id", "scheduled_report_tasks", ["tenant_id"])
    op.create_index("ix_scheduled_report_due", "scheduled_report_tasks", ["status", "next_run_at"])
    op.create_table(
        "scheduled_report_runs",
        sa.Column("id", sa.BigInteger(), sa.Identity(), primary_key=True),
        sa.Column("tenant_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("task_id", sa.Integer(), nullable=False),
        sa.Column("status", sa.String(20), nullable=False),
        sa.Column("trigger", sa.String(20), nullable=False),
        sa.Column("report_note_id", sa.Integer()),
        sa.Column("error_code", sa.String(80)),
        sa.Column("error_message", sa.Text()),
        sa.Column("started_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("finished_at", sa.DateTime(timezone=True)),
        sa.ForeignKeyConstraint(["tenant_id"], ["tenants.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["task_id"], ["scheduled_report_tasks.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(["report_note_id"], ["notes.id"], ondelete="SET NULL"),
    )
    op.create_index("ix_scheduled_report_runs_tenant_id", "scheduled_report_runs", ["tenant_id"])
    op.create_index("ix_scheduled_report_runs_task_id", "scheduled_report_runs", ["task_id"])
    _rls("scheduled_report_tasks")
    _rls("scheduled_report_runs")


def downgrade() -> None:
    op.drop_table("scheduled_report_runs")
    op.drop_table("scheduled_report_tasks")
