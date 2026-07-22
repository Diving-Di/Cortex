import os
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.skipif(
    os.getenv("RUN_POSTGRES_TESTS") != "1", reason="requires isolated PostgreSQL test database"
)


def test_personal_tenant_notes_revisions_and_rls() -> None:
    from app.core.database import SessionLocal
    from app.main import app
    from app.models import AuditLog, Note, Tenant, User

    client = TestClient(app)
    suffix = uuid4().hex[:10]
    credentials = []
    user_ids = []
    for marker in ("a", "b"):
        username = f"notes_{suffix}_{marker}"
        password = "correct-horse-battery"
        assert (
            client.post(
                "/api/v1/auth/register",
                json={
                    "username": username,
                    "email": f"{username}@example.invalid",
                    "password": password,
                },
            ).status_code
            == 201
        )
        login = client.post("/api/v1/auth/login", json={"username": username, "password": password})
        credentials.append({"Authorization": f"Token {login.json()['token']}"})
    try:
        tenants = [client.get("/api/v1/tenant", headers=headers).json() for headers in credentials]
        assert tenants[0]["id"] != tenants[1]["id"]
        assert all(item["note_count"] == 0 for item in tenants)
        assert (
            client.post(
                "/api/v1/tenant", headers=credentials[0], json={"name": "second"}
            ).status_code
            == 405
        )
        renamed = client.patch("/api/v1/tenant", headers=credentials[0], json={"name": "我的空间"})
        assert renamed.status_code == 200 and renamed.json()["name"] == "我的空间"

        created = []
        for note_type in ("normal", "daily", "weekly", "monthly"):
            body = {"type": note_type, "title": note_type, "content": "第一版"}
            if note_type != "normal":
                body["note_date"] = "2026-07-22"
            response = client.post("/api/v1/notes", headers=credentials[0], json=body)
            assert response.status_code == 201, response.text
            created.append(response.json())
        duplicate = client.post(
            "/api/v1/notes",
            headers=credentials[0],
            json={"type": "daily", "title": "duplicate", "content": "x", "note_date": "2026-07-22"},
        )
        assert duplicate.status_code == 409
        injected = client.post(
            "/api/v1/notes",
            headers=credentials[1],
            json={"type": "normal", "title": "injected", "tenant_id": tenants[0]["id"]},
        )
        assert injected.status_code == 422

        note_id = created[0]["id"]
        assert client.get(f"/api/v1/notes/{note_id}", headers=credentials[1]).status_code == 404
        assert (
            client.patch(
                f"/api/v1/notes/{note_id}", headers=credentials[0], json={"content": "第二版"}
            ).status_code
            == 200
        )
        revisions = client.get(f"/api/v1/notes/{note_id}/revisions", headers=credentials[0]).json()
        assert revisions[0]["content"] == "第一版"
        restored = client.post(
            f"/api/v1/notes/{note_id}/revisions/{revisions[0]['id']}/restore",
            headers=credentials[0],
        )
        assert restored.json()["content"] == "第一版"
        page = client.get("/api/v1/notes?page=1&page_size=2", headers=credentials[0]).json()
        assert page["total"] == 4 and len(page["items"]) == 2

        with SessionLocal.begin() as db:
            users = db.query(User).filter(User.username.like(f"notes_{suffix}_%")).all()
            user_ids = [user.id for user in users]
            tenant_id = db.query(Tenant.id).filter(Tenant.user_id == user_ids[0]).scalar()
            assert db.query(Note).count() == 0
            db.execute(
                __import__("sqlalchemy").text(
                    "SELECT set_config('app.current_tenant_id', :id, true)"
                ),
                {"id": str(tenant_id)},
            )
            assert db.query(Note).count() == 4
            assert db.query(AuditLog).filter(AuditLog.tenant_id == tenant_id).count() >= 6

        assert client.delete("/api/v1/tenant", headers=credentials[0]).status_code == 204
        assert client.get("/api/v1/notes", headers=credentials[0]).status_code == 403
        assert client.post("/api/v1/tenant/restore", headers=credentials[0]).status_code == 200
        assert client.get("/api/v1/notes", headers=credentials[0]).status_code == 200
    finally:
        with SessionLocal.begin() as db:
            ids = [
                row[0]
                for row in db.query(User.id).filter(User.username.like(f"notes_{suffix}_%")).all()
            ]
            if ids:
                db.query(Tenant).filter(Tenant.user_id.in_(ids)).delete(synchronize_session=False)
                db.query(User).filter(User.id.in_(ids)).delete(synchronize_session=False)
