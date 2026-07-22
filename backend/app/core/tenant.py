from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from sqlalchemy import text
from sqlalchemy.orm import Session
from fastapi import Depends, HTTPException, status

from ..database import get_db
from ..models import Tenant, User
from ..security import get_current_user


@dataclass(frozen=True)
class TenantContext:
    user_id: int
    tenant_id: UUID


def apply_tenant_context(session: Session, context: TenantContext) -> None:
    """Set PostgreSQL transaction-local RLS context from trusted server data."""
    session.execute(
        text("SELECT set_config('app.current_tenant_id', :tenant_id, true)"),
        {"tenant_id": str(context.tenant_id)},
    )


def get_tenant_context(db: Session = Depends(get_db), user: User = Depends(get_current_user)) -> TenantContext:
    tenant = db.query(Tenant).filter(Tenant.user_id == user.id, Tenant.status == "active", Tenant.deleted_at.is_(None)).one_or_none()
    if tenant is None:
        raise HTTPException(status_code=status.HTTP_403_FORBIDDEN, detail="Personal tenant is unavailable.")
    context = TenantContext(user_id=user.id, tenant_id=tenant.id)
    apply_tenant_context(db, context)
    return context
