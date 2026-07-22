from datetime import datetime, timezone
from fastapi import APIRouter, Depends, Response, status
from sqlalchemy import func
from sqlalchemy.orm import Session

from ..core.database import get_db
from ..core.tenant import TenantContext, get_tenant_context
from ..models import AiUsageRecord, Note, Tenant, User
from ..schemas import TenantOut, TenantUpdate
from ..security import get_current_user
from ..services.audit_service import record_audit

router = APIRouter(prefix="/api/v1/tenant", tags=["tenant"])

@router.patch("", response_model=TenantOut)
def update_tenant(payload: TenantUpdate, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> TenantOut:
    tenant = db.query(Tenant).filter(Tenant.id == context.tenant_id).one()
    tenant.name = payload.name.strip()
    if not tenant.name:
        from ..core.exceptions import AppError
        raise AppError("TENANT_NAME_REQUIRED", "个人空间名称不能为空", 422)
    record_audit(db, context, "tenant.update", "tenant", tenant.id)
    note_count = db.query(func.count(Note.id)).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None)).scalar() or 0
    tokens = db.query(func.coalesce(func.sum(AiUsageRecord.input_tokens + AiUsageRecord.output_tokens), 0)).filter(AiUsageRecord.tenant_id == context.tenant_id).scalar() or 0
    return TenantOut(id=str(tenant.id), name=tenant.name, status=tenant.status, note_quota=tenant.note_quota, note_count=note_count, attachment_quota_bytes=tenant.attachment_quota_bytes, ai_token_quota=tenant.ai_token_quota, ai_tokens_used=tokens)

@router.get("", response_model=TenantOut)
def current_tenant(db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> TenantOut:
    tenant = db.query(Tenant).filter(Tenant.id == context.tenant_id).one()
    note_count = db.query(func.count(Note.id)).filter(Note.tenant_id == context.tenant_id, Note.deleted_at.is_(None)).scalar() or 0
    tokens = db.query(func.coalesce(func.sum(AiUsageRecord.input_tokens + AiUsageRecord.output_tokens), 0)).filter(AiUsageRecord.tenant_id == context.tenant_id).scalar() or 0
    return TenantOut(id=str(tenant.id), name=tenant.name, status=tenant.status, note_quota=tenant.note_quota, note_count=note_count, attachment_quota_bytes=tenant.attachment_quota_bytes, ai_token_quota=tenant.ai_token_quota, ai_tokens_used=tokens)

@router.delete("", status_code=status.HTTP_204_NO_CONTENT, response_class=Response)
def delete_tenant(db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> Response:
    tenant = db.query(Tenant).filter(Tenant.id == context.tenant_id).one()
    record_audit(db, context, "tenant.soft_delete", "tenant", tenant.id)
    tenant.status = "deleted"
    tenant.deleted_at = datetime.now(timezone.utc)
    return Response(status_code=204)

@router.post("/restore", response_model=TenantOut)
def restore_tenant(db: Session = Depends(get_db), user: User = Depends(get_current_user)) -> TenantOut:
    tenant = db.query(Tenant).filter(Tenant.user_id == user.id, Tenant.status == "deleted").one_or_none()
    if tenant is None:
        from ..core.exceptions import AppError
        raise AppError("TENANT_NOT_DELETED", "个人空间不处于可恢复状态", 409)
    tenant.status = "active"
    tenant.deleted_at = None
    db.flush()
    context = TenantContext(user_id=user.id, tenant_id=tenant.id)
    from ..core.tenant import apply_tenant_context
    apply_tenant_context(db, context)
    record_audit(db, context, "tenant.restore", "tenant", tenant.id)
    note_count = db.query(func.count(Note.id)).filter(Note.tenant_id == tenant.id, Note.deleted_at.is_(None)).scalar() or 0
    tokens = db.query(func.coalesce(func.sum(AiUsageRecord.input_tokens + AiUsageRecord.output_tokens), 0)).filter(AiUsageRecord.tenant_id == tenant.id).scalar() or 0
    return TenantOut(id=str(tenant.id), name=tenant.name, status=tenant.status, note_quota=tenant.note_quota, note_count=note_count, attachment_quota_bytes=tenant.attachment_quota_bytes, ai_token_quota=tenant.ai_token_quota, ai_tokens_used=tokens)
