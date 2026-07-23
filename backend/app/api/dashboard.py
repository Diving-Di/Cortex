from fastapi import APIRouter, Depends, Query
from sqlalchemy.orm import Session

from ..core.database import get_db
from ..core.tenant import TenantContext, get_tenant_context
from ..services.dashboard_service import dashboard_summary

router = APIRouter(prefix="/api/dashboard", tags=["dashboard"])


@router.get("")
def get_dashboard(
    timezone: str = Query(default="Asia/Shanghai", max_length=64),
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> dict:
    return dashboard_summary(db, context, timezone)
