import os
from datetime import datetime, timedelta, timezone
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.skipif(
    os.getenv("RUN_POSTGRES_TESTS") != "1", reason="requires isolated PostgreSQL test database"
)


def test_token_lifecycle_and_user_resource_isolation() -> None:
    from app.core.database import SessionLocal
    from app.main import app
    from app.core.tenant import TenantContext, apply_tenant_context
    from app.models import AuthToken, Conversation, DiaryEntry, Tenant, User
    from app.security import hash_token

    client = TestClient(app)
    suffix = uuid4().hex[:10]
    names = [f"user_{suffix}_a", f"user_{suffix}_b"]
    tokens: list[str] = []
    user_ids: list[int] = []
    try:
        for index, name in enumerate(names):
            response = client.post(
                "/api/v1/auth/register",
                json={
                    "username": name,
                    "email": f"{name}@example.invalid",
                    "password": "correct-horse-battery",
                },
            )
            assert response.status_code == 201
            response = client.post(
                "/api/v1/auth/login", json={"username": name, "password": "correct-horse-battery"}
            )
            assert response.status_code == 200
            tokens.append(response.json()["token"])

        with SessionLocal.begin() as db:
            users = db.query(User).filter(User.username.in_(names)).order_by(User.username).all()
            user_ids = [user.id for user in users]
            tenant_ids = [
                db.query(Tenant.id).filter(Tenant.user_id == user.id).scalar() for user in users
            ]
            assert db.query(AuthToken).filter(AuthToken.token_hash == tokens[0]).count() == 0
            assert (
                db.query(AuthToken).filter(AuthToken.token_hash == hash_token(tokens[0])).count()
                == 1
            )
            apply_tenant_context(db, TenantContext(user_ids[0], tenant_ids[0]))
            conversation = Conversation(
                tenant_id=tenant_ids[0], user_id=user_ids[0], title="private"
            )
            db.add(conversation)
            db.flush()
            conversation_id = conversation.id
            db.add(DiaryEntry(tenant_id=tenant_ids[0], user_id=user_ids[0], content="owner-only"))
        with SessionLocal.begin() as db:
            apply_tenant_context(db, TenantContext(user_ids[1], tenant_ids[1]))
            db.add(DiaryEntry(tenant_id=tenant_ids[1], user_id=user_ids[1], content="other-only"))

        own_headers = {"Authorization": f"Token {tokens[0]}"}
        other_headers = {"Authorization": f"Token {tokens[1]}"}
        assert (
            client.get(
                f"/api/chat/conversations/{conversation_id}/", headers=other_headers
            ).status_code
            == 404
        )
        own_diary = client.get("/api/diary/", headers=own_headers).json()
        assert [entry["content"] for entry in own_diary] == ["owner-only"]

        assert client.post("/api/v1/auth/logout", headers=own_headers).status_code == 204
        assert client.get("/api/diary/", headers=own_headers).status_code == 401

        with SessionLocal.begin() as db:
            token = db.query(AuthToken).filter(AuthToken.token_hash == hash_token(tokens[1])).one()
            token.expires_at = datetime.now(timezone.utc) - timedelta(seconds=1)
        assert client.get("/api/diary/", headers=other_headers).status_code == 401
    finally:
        if user_ids:
            with SessionLocal.begin() as db:
                db.query(User).filter(User.id.in_(user_ids)).delete(synchronize_session=False)
