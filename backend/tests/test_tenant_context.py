from unittest.mock import Mock
from uuid import UUID

from app.core.tenant import TenantContext, apply_tenant_context


def test_tenant_context_is_transaction_local_and_parameterized() -> None:
    session = Mock()
    context = TenantContext(user_id=7, tenant_id=UUID("12345678-1234-5678-1234-567812345678"))
    apply_tenant_context(session, context)
    statement, parameters = session.execute.call_args.args
    assert "set_config" in str(statement)
    assert parameters == {"tenant_id": "12345678-1234-5678-1234-567812345678"}
