"""Tenant scope all business data and complete the notes core."""
from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0003_tenant_notes_core"
down_revision = "0002_secure_auth_tokens"
branch_labels = None
depends_on = None

TENANT_TABLES = ("notes", "conversations", "messages", "diary_entries", "note_revisions", "ai_usage_records", "audit_logs")


def _enable_rls(table: str) -> None:
    op.execute(f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY")
    op.execute(f"ALTER TABLE {table} FORCE ROW LEVEL SECURITY")
    op.execute(f"CREATE POLICY {table}_tenant_isolation ON {table} USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)")


def upgrade() -> None:
    op.execute("CREATE EXTENSION IF NOT EXISTS pg_trgm")
    op.add_column("tenants", sa.Column("note_quota", sa.Integer(), server_default="10000", nullable=False))
    op.add_column("tenants", sa.Column("attachment_quota_bytes", sa.BigInteger(), server_default=str(1024 * 1024 * 1024), nullable=False))
    op.add_column("tenants", sa.Column("ai_token_quota", sa.BigInteger(), server_default="1000000", nullable=False))
    op.add_column("tenants", sa.Column("deleted_at", sa.DateTime(timezone=True)))
    op.execute("INSERT INTO tenants (id, user_id, name) SELECT gen_random_uuid(), u.id, u.username || ' 的个人空间' FROM users u LEFT JOIN tenants t ON t.user_id=u.id WHERE t.id IS NULL")
    op.create_unique_constraint("uq_tenants_id_user", "tenants", ["id", "user_id"])

    for table in ("conversations", "diary_entries"):
        op.add_column(table, sa.Column("tenant_id", postgresql.UUID(as_uuid=True)))
        op.execute(f"UPDATE {table} b SET tenant_id=t.id FROM tenants t WHERE t.user_id=b.user_id")
        op.execute(f"DELETE FROM {table} WHERE tenant_id IS NULL")
        op.alter_column(table, "tenant_id", nullable=False)
        op.create_foreign_key(f"fk_{table}_tenant", table, "tenants", ["tenant_id"], ["id"], ondelete="CASCADE")
        op.create_index(f"ix_{table}_tenant_id", table, ["tenant_id"])
        op.create_foreign_key(f"fk_{table}_tenant_user", table, "tenants", ["tenant_id", "user_id"], ["id", "user_id"], ondelete="CASCADE")

    op.create_unique_constraint("uq_conversations_tenant_id_id", "conversations", ["tenant_id", "id"])

    op.add_column("messages", sa.Column("tenant_id", postgresql.UUID(as_uuid=True)))
    op.execute("UPDATE messages m SET tenant_id=c.tenant_id FROM conversations c WHERE c.id=m.conversation_id")
    op.execute("DELETE FROM messages WHERE tenant_id IS NULL")
    op.alter_column("messages", "tenant_id", nullable=False)
    op.create_foreign_key("fk_messages_tenant", "messages", "tenants", ["tenant_id"], ["id"], ondelete="CASCADE")
    op.create_index("ix_messages_tenant_id", "messages", ["tenant_id"])
    op.create_foreign_key("fk_messages_tenant_conversation", "messages", "conversations", ["tenant_id", "conversation_id"], ["tenant_id", "id"], ondelete="CASCADE")

    op.drop_constraint("uq_notes_tenant_period", "notes", type_="unique")
    op.create_check_constraint("ck_notes_type", "notes", "type IN ('normal','daily','weekly','monthly')")
    op.create_index("uq_notes_tenant_period", "notes", ["tenant_id", "type", "note_date"], unique=True, postgresql_where=sa.text("type IN ('daily','weekly','monthly') AND deleted_at IS NULL"))
    op.create_unique_constraint("uq_notes_tenant_id_id", "notes", ["tenant_id", "id"])
    op.create_foreign_key("fk_notes_tenant_creator", "notes", "tenants", ["tenant_id", "created_by"], ["id", "user_id"], ondelete="CASCADE")
    op.create_foreign_key("fk_notes_tenant_updater", "notes", "tenants", ["tenant_id", "updated_by"], ["id", "user_id"], ondelete="CASCADE")

    op.create_table("note_revisions",
        sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True),
        sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False),
        sa.Column("note_id", sa.Integer(), nullable=False),
        sa.Column("created_by", sa.Integer(), sa.ForeignKey("users.id"), nullable=False),
        sa.Column("content", sa.Text(), nullable=False), sa.Column("reason", sa.String(40), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id", "note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE", name="fk_revisions_tenant_note"),
        sa.ForeignKeyConstraint(["tenant_id", "created_by"], ["tenants.id", "tenants.user_id"], ondelete="CASCADE", name="fk_revisions_tenant_user"))
    op.create_index("ix_note_revisions_tenant_id", "note_revisions", ["tenant_id"])
    op.create_index("ix_note_revisions_note_id", "note_revisions", ["note_id"])
    op.create_table("ai_usage_records",
        sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True), sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False), sa.Column("request_type", sa.String(40), nullable=False),
        sa.Column("input_tokens", sa.Integer(), server_default="0", nullable=False), sa.Column("output_tokens", sa.Integer(), server_default="0", nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id", "user_id"], ["tenants.id", "tenants.user_id"], ondelete="CASCADE", name="fk_ai_usage_tenant_user"))
    op.create_index("ix_ai_usage_records_tenant_id", "ai_usage_records", ["tenant_id"])
    op.create_table("audit_logs",
        sa.Column("id", sa.BigInteger(), sa.Identity(), primary_key=True), sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False),
        sa.Column("user_id", sa.Integer(), sa.ForeignKey("users.id"), nullable=False), sa.Column("action", sa.String(80), nullable=False),
        sa.Column("resource_type", sa.String(40), nullable=False), sa.Column("resource_id", sa.String(64)),
        sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.ForeignKeyConstraint(["tenant_id", "user_id"], ["tenants.id", "tenants.user_id"], ondelete="CASCADE", name="fk_audit_tenant_user"))
    op.create_index("ix_audit_logs_tenant_id", "audit_logs", ["tenant_id"])

    op.execute("DROP POLICY notes_tenant_isolation ON notes")
    for table in TENANT_TABLES:
        _enable_rls(table)


def downgrade() -> None:
    for table in TENANT_TABLES:
        op.execute(f"DROP POLICY IF EXISTS {table}_tenant_isolation ON {table}")
        if table in ("notes", "conversations", "messages", "diary_entries"):
            op.execute(f"ALTER TABLE {table} NO FORCE ROW LEVEL SECURITY")
            op.execute(f"ALTER TABLE {table} DISABLE ROW LEVEL SECURITY")
    op.drop_table("audit_logs")
    op.drop_table("ai_usage_records")
    op.drop_table("note_revisions")
    op.execute("ALTER TABLE notes DROP CONSTRAINT IF EXISTS fk_notes_tenant_updater")
    op.execute("ALTER TABLE notes DROP CONSTRAINT IF EXISTS fk_notes_tenant_creator")
    op.drop_constraint("uq_notes_tenant_id_id", "notes", type_="unique")
    op.drop_index("uq_notes_tenant_period", table_name="notes")
    op.drop_constraint("ck_notes_type", "notes", type_="check")
    op.create_unique_constraint("uq_notes_tenant_period", "notes", ["tenant_id", "type", "note_date"])
    op.execute("ALTER TABLE notes ENABLE ROW LEVEL SECURITY")
    op.execute("ALTER TABLE notes FORCE ROW LEVEL SECURITY")
    op.execute("CREATE POLICY notes_tenant_isolation ON notes USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid) WITH CHECK (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::uuid)")
    op.execute("ALTER TABLE messages DROP CONSTRAINT IF EXISTS fk_messages_tenant_conversation")
    op.execute("ALTER TABLE conversations DROP CONSTRAINT IF EXISTS uq_conversations_tenant_id_id")
    for table in ("messages", "diary_entries", "conversations"):
        if table in ("diary_entries", "conversations"):
            op.execute(f"ALTER TABLE {table} DROP CONSTRAINT IF EXISTS fk_{table}_tenant_user")
        op.drop_index(f"ix_{table}_tenant_id", table_name=table)
        op.drop_constraint(f"fk_{table}_tenant", table, type_="foreignkey")
        op.drop_column(table, "tenant_id")
    op.execute("ALTER TABLE tenants DROP CONSTRAINT IF EXISTS uq_tenants_id_user")
    op.drop_column("tenants", "deleted_at")
    op.drop_column("tenants", "ai_token_quota")
    op.drop_column("tenants", "attachment_quota_bytes")
    op.drop_column("tenants", "note_quota")
