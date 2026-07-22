from sqlalchemy.orm import Session
from ..core.tenant import TenantContext
from ..models import AuditLog

def record_audit(db: Session, context: TenantContext, action: str, resource_type: str, resource_id: object = None) -> None:
    db.add(AuditLog(tenant_id=context.tenant_id, user_id=context.user_id, action=action, resource_type=resource_type, resource_id=str(resource_id) if resource_id is not None else None))
