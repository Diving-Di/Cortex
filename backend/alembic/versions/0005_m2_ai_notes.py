"""M2 report and memory source citations."""

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0005_m2_ai_notes"
down_revision = "0004_content_search_ai"
branch_labels = None
depends_on = None


def _rls(table: str) -> None:
    op.execute(f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY")
    op.execute(f"ALTER TABLE {table} FORCE ROW LEVEL SECURITY")
    op.execute(
        f"CREATE POLICY {table}_tenant_isolation ON {table} USING (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid)"
    )


def upgrade() -> None:
    op.create_table(
        "report_sources",
        sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True),
        sa.Column("tenant_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("report_note_id", sa.Integer(), nullable=False),
        sa.Column("source_note_id", sa.Integer(), nullable=False),
        sa.Column("rank", sa.Integer(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id"], ["tenants.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(
            ["tenant_id", "report_note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE"
        ),
        sa.ForeignKeyConstraint(
            ["tenant_id", "source_note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE"
        ),
        sa.UniqueConstraint("report_note_id", "source_note_id"),
    )
    op.create_index("ix_report_sources_tenant_id", "report_sources", ["tenant_id"])
    op.create_index("ix_report_sources_report_note_id", "report_sources", ["report_note_id"])
    op.create_table(
        "message_sources",
        sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True),
        sa.Column("tenant_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("message_id", sa.Integer(), nullable=False),
        sa.Column("note_id", sa.Integer(), nullable=False),
        sa.Column("snippet", sa.Text(), nullable=False),
        sa.Column("relevance", sa.Integer(), server_default="0", nullable=False),
        sa.Column("rank", sa.Integer(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id"], ["tenants.id"], ondelete="CASCADE"),
        sa.ForeignKeyConstraint(
            ["tenant_id", "message_id"], ["messages.tenant_id", "messages.id"], ondelete="CASCADE"
        ),
        sa.ForeignKeyConstraint(
            ["tenant_id", "note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE"
        ),
    )
    op.create_index("ix_message_sources_tenant_id", "message_sources", ["tenant_id"])
    op.create_index("ix_message_sources_message_id", "message_sources", ["message_id"])
    for table in ("report_sources", "message_sources"):
        _rls(table)


def downgrade() -> None:
    op.drop_table("message_sources")
    op.drop_table("report_sources")
