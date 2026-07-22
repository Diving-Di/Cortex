"""Hash, expire and revoke authentication tokens.

Existing development tokens are invalidated because their raw values cannot be
safely transformed without retaining a reversible credential.
"""

from alembic import op
import sqlalchemy as sa

revision = "0002_secure_auth_tokens"
down_revision = "0001_engineering_baseline"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.execute("DELETE FROM auth_tokens")
    op.drop_column("auth_tokens", "key")
    op.add_column("auth_tokens", sa.Column("id", sa.Integer(), sa.Identity(), nullable=False))
    op.add_column("auth_tokens", sa.Column("token_hash", sa.String(64), nullable=False))
    op.add_column(
        "auth_tokens", sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False)
    )
    op.add_column("auth_tokens", sa.Column("revoked_at", sa.DateTime(timezone=True)))
    op.add_column("auth_tokens", sa.Column("last_used_at", sa.DateTime(timezone=True)))
    op.create_primary_key("pk_auth_tokens", "auth_tokens", ["id"])
    op.create_index("ix_auth_tokens_token_hash", "auth_tokens", ["token_hash"], unique=True)


def downgrade() -> None:
    op.execute("DELETE FROM auth_tokens")
    op.drop_index("ix_auth_tokens_token_hash", table_name="auth_tokens")
    op.drop_constraint("pk_auth_tokens", "auth_tokens", type_="primary")
    op.drop_column("auth_tokens", "last_used_at")
    op.drop_column("auth_tokens", "revoked_at")
    op.drop_column("auth_tokens", "expires_at")
    op.drop_column("auth_tokens", "token_hash")
    op.drop_column("auth_tokens", "id")
    op.add_column("auth_tokens", sa.Column("key", sa.String(64), nullable=False))
    op.create_primary_key("auth_tokens_pkey", "auth_tokens", ["key"])
