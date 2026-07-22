import io, json, os, zipfile
from uuid import uuid4
import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.skipif(
    os.getenv("RUN_POSTGRES_TESTS") != "1", reason="requires isolated PostgreSQL test database"
)


def test_tags_attachments_search_backup_restore_and_ai_fallback() -> None:
    from app.core.database import SessionLocal
    from app.main import app
    from app.models import Tenant, User

    client = TestClient(app)
    suffix = uuid4().hex[:10]
    headers = []
    for marker in ("a", "b"):
        name = f"content_{suffix}_{marker}"
        password = "correct-horse-battery"
        assert (
            client.post(
                "/api/v1/auth/register",
                json={"username": name, "email": f"{name}@example.invalid", "password": password},
            ).status_code
            == 201
        )
        token = client.post(
            "/api/v1/auth/login", json={"username": name, "password": password}
        ).json()["token"]
        headers.append({"Authorization": f"Token {token}"})
    try:
        note = client.post(
            "/api/v1/notes",
            headers=headers[0],
            json={
                "type": "normal",
                "title": "中文旅行记录",
                "content": "今天去了杭州西湖，天气很好。",
            },
        ).json()
        note_id = note["id"]
        tag = client.post(
            "/api/v1/tags", headers=headers[0], json={"name": "旅行", "color": "#1677ff"}
        ).json()
        assert (
            client.put(
                f"/api/v1/notes/{note_id}/tags", headers=headers[0], json={"tag_ids": [tag["id"]]}
            ).status_code
            == 200
        )
        search = client.get(
            "/api/v1/search", headers=headers[0], params={"q": "西湖", "tag_id": tag["id"]}
        ).json()
        assert search["total"] == 1 and search["items"][0]["id"] == note_id
        assert (
            client.get("/api/v1/search", headers=headers[1], params={"q": "西湖"}).json()["total"]
            == 0
        )

        upload = client.post(
            "/api/v1/attachments",
            headers=headers[0],
            params={"note_id": note_id},
            files={"file": ("记录.md", "附件内容".encode(), "text/markdown")},
        )
        assert upload.status_code == 201, upload.text
        attachment = upload.json()
        assert (
            client.get(f"/api/v1/attachments/{attachment['id']}", headers=headers[1]).status_code
            == 404
        )
        assert (
            client.get(f"/api/v1/attachments/{attachment['id']}", headers=headers[0]).content
            == "附件内容".encode()
        )
        forged = client.post(
            "/api/v1/attachments",
            headers=headers[0],
            params={"note_id": note_id},
            files={"file": ("fake.png", b"not-a-png", "image/png")},
        )
        assert forged.status_code == 422
        empty = client.post(
            "/api/v1/attachments",
            headers=headers[0],
            params={"note_id": note_id},
            files={"file": ("empty.txt", b"", "text/plain")},
        )
        assert empty.status_code == 422
        oversized = client.post(
            "/api/v1/attachments",
            headers=headers[0],
            params={"note_id": note_id},
            files={"file": ("large.txt", b"x" * (20 * 1024 * 1024 + 1), "text/plain")},
        )
        assert oversized.status_code == 413

        exported = client.post("/api/v1/exports/markdown", headers=headers[0])
        assert exported.status_code == 200
        with zipfile.ZipFile(io.BytesIO(exported.content)) as z:
            assert any(z.read(name).decode().startswith("# 中文旅行记录") for name in z.namelist())
        backup = client.post("/api/v1/backups", headers=headers[0])
        assert backup.status_code == 200
        restored = client.post(
            "/api/v1/backups/restore",
            headers=headers[1],
            files={"file": ("backup.zip", backup.content, "application/zip")},
        )
        assert restored.status_code == 200 and restored.json() == {
            "notes": 1,
            "tags": 1,
            "attachments": 1,
        }
        assert (
            client.get("/api/v1/search", headers=headers[1], params={"q": "西湖"}).json()["total"]
            == 1
        )

        bad = io.BytesIO()
        with zipfile.ZipFile(bad, "w") as z:
            z.writestr("../escape.txt", "bad")
            z.writestr("data.json", "{}")
            z.writestr("manifest.json", "{}")
        assert (
            client.post(
                "/api/v1/backups/restore",
                headers=headers[1],
                files={"file": ("bad.zip", bad.getvalue(), "application/zip")},
            ).status_code
            == 422
        )
        ai = client.post("/api/v1/ai/stream", headers=headers[0], json={"prompt": "hello"})
        assert ai.status_code == 503
        settings = client.get("/api/v1/settings/ai", headers=headers[0]).json()
        assert settings["api_key"] is None and settings["configured"] is False
        assert client.get("/api/v1/notes", headers=headers[0]).status_code == 200
        for owner in headers:
            notes = client.get("/api/v1/notes", headers=owner).json()["items"]
            for n in notes:
                for item in client.get(f"/api/v1/attachments/note/{n['id']}", headers=owner).json():
                    client.delete(f"/api/v1/attachments/{item['id']}", headers=owner)
    finally:
        with SessionLocal.begin() as db:
            ids = [
                x[0]
                for x in db.query(User.id).filter(User.username.like(f"content_{suffix}_%")).all()
            ]
            if ids:
                db.query(Tenant).filter(Tenant.user_id.in_(ids)).delete(synchronize_session=False)
                db.query(User).filter(User.id.in_(ids)).delete(synchronize_session=False)
