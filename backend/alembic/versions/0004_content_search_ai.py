"""Tags, secure attachments, trigram indexes and AI providers."""
from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

revision = "0004_content_search_ai"
down_revision = "0003_tenant_notes_core"
branch_labels = None
depends_on = None

def _rls(table: str) -> None:
    op.execute(f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY")
    op.execute(f"ALTER TABLE {table} FORCE ROW LEVEL SECURITY")
    op.execute(f"CREATE POLICY {table}_tenant_isolation ON {table} USING (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid) WITH CHECK (tenant_id=NULLIF(current_setting('app.current_tenant_id', true),'')::uuid)")

def upgrade() -> None:
    op.create_table("tags", sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True), sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False), sa.Column("name", sa.String(80), nullable=False), sa.Column("color", sa.String(20)), sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False))
    op.create_index("ix_tags_tenant_id", "tags", ["tenant_id"]); op.create_index("uq_tags_tenant_name", "tags", ["tenant_id", "name"], unique=True)
    op.create_unique_constraint("uq_tags_tenant_id_id", "tags", ["tenant_id", "id"])
    op.create_table("note_tags", sa.Column("tenant_id", postgresql.UUID(as_uuid=True), nullable=False), sa.Column("note_id", sa.Integer(), nullable=False), sa.Column("tag_id", sa.Integer(), nullable=False), sa.PrimaryKeyConstraint("tenant_id", "note_id", "tag_id"), sa.ForeignKeyConstraint(["tenant_id", "note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE"), sa.ForeignKeyConstraint(["tenant_id", "tag_id"], ["tags.tenant_id", "tags.id"], ondelete="CASCADE"))
    op.create_table("attachments", sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True), sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False), sa.Column("uploaded_by", sa.Integer(), sa.ForeignKey("users.id"), nullable=False), sa.Column("note_id", sa.Integer(), nullable=False), sa.Column("original_name", sa.String(255), nullable=False), sa.Column("stored_path", sa.String(500), unique=True, nullable=False), sa.Column("mime_type", sa.String(120), nullable=False), sa.Column("size", sa.BigInteger(), nullable=False), sa.Column("sha256", sa.String(64), nullable=False), sa.Column("created_at", sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False), sa.ForeignKeyConstraint(["tenant_id", "note_id"], ["notes.tenant_id", "notes.id"], ondelete="CASCADE"), sa.ForeignKeyConstraint(["tenant_id", "uploaded_by"], ["tenants.id", "tenants.user_id"], ondelete="CASCADE"))
    op.create_index("ix_attachments_tenant_id", "attachments", ["tenant_id"]); op.create_index("ix_attachments_note_id", "attachments", ["note_id"])
    op.create_table("ai_providers", sa.Column("id", sa.Integer(), sa.Identity(), primary_key=True), sa.Column("tenant_id", postgresql.UUID(as_uuid=True), sa.ForeignKey("tenants.id", ondelete="CASCADE"), nullable=False), sa.Column("display_name", sa.String(100), nullable=False), sa.Column("base_url", sa.String(500), nullable=False), sa.Column("default_model", sa.String(120), nullable=False), sa.Column("capabilities", sa.String(255), server_default="chat,stream", nullable=False), sa.Column("enabled", sa.Integer(), server_default="1", nullable=False))
    op.create_index("ix_ai_providers_tenant_id", "ai_providers", ["tenant_id"])
    op.add_column("ai_usage_records", sa.Column("model", sa.String(120)))
    op.add_column("ai_usage_records", sa.Column("duration_ms", sa.Integer()))
    op.add_column("ai_usage_records", sa.Column("status", sa.String(30), server_default="success", nullable=False))
    op.add_column("ai_usage_records", sa.Column("error_code", sa.String(80)))
    op.create_index("ix_notes_title_trgm", "notes", ["title"], postgresql_using="gin", postgresql_ops={"title": "gin_trgm_ops"})
    op.create_index("ix_notes_content_trgm", "notes", ["content"], postgresql_using="gin", postgresql_ops={"content": "gin_trgm_ops"})
    for table in ("tags", "note_tags", "attachments", "ai_providers"): _rls(table)

def downgrade() -> None:
    op.drop_index("ix_notes_content_trgm", table_name="notes"); op.drop_index("ix_notes_title_trgm", table_name="notes")
    for col in ("error_code", "status", "duration_ms", "model"): op.drop_column("ai_usage_records", col)
    for table in ("ai_providers", "attachments", "note_tags", "tags"): op.drop_table(table)
