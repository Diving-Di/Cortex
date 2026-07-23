import os
from types import SimpleNamespace
from uuid import uuid4
import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.skipif(
    os.getenv("RUN_POSTGRES_TESTS") != "1", reason="requires isolated PostgreSQL test database"
)


class FakeAIClient:
    def __init__(self, *args, **kwargs):
        pass

    async def stream_chat(self, model, messages):
        prompt = messages[0]["content"]
        if "仅输出 JSON" in prompt:
            yield '{"title":"整理后的西湖记录","summary":"杭州旅行","content":"# 西湖\\n\\n天气很好"}'
        elif "Markdown 报告" in prompt:
            yield "# 日报\n\n游览了西湖 [#1]"
        else:
            yield "根据记录，你去了杭州西湖 [#1]。"


def test_m2_preview_confirm_report_and_memory_citations(monkeypatch):
    import app.api.m2 as m2
    from app.core.database import SessionLocal
    from app.main import app
    from app.models import Message, NoteRevision, Tenant, User

    monkeypatch.setattr(m2, "OpenAICompatibleClient", FakeAIClient)
    monkeypatch.setattr(
        m2,
        "get_settings",
        lambda: SimpleNamespace(
            ai={"api_key": "test-only", "base_url": "http://invalid", "model": "fake"}
        ),
    )
    client = TestClient(app)
    suffix = uuid4().hex[:10]
    username = f"m2_{suffix}"
    other_username = f"m2_other_{suffix}"
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
    token = client.post(
        "/api/v1/auth/login", json={"username": username, "password": password}
    ).json()["token"]
    headers = {"Authorization": f"Token {token}"}
    try:
        source = client.post(
            "/api/v1/notes",
            headers=headers,
            json={
                "type": "normal",
                "title": "杭州旅行",
                "content": "今天去了杭州西湖，天气很好。",
                "note_date": "2026-07-23",
            },
        ).json()
        dashboard = client.get("/api/dashboard?timezone=Asia/Shanghai", headers=headers)
        assert dashboard.status_code == 200
        dashboard_data = dashboard.json()
        assert dashboard_data["statistics"]["notes"] == 1
        assert dashboard_data["recent_notes"][0]["id"] == source["id"]
        assert dashboard_data["timezone"] == "Asia/Shanghai"
        organized = client.post(
            "/api/v1/ai/organize", headers=headers, json={"content": "西湖天气很好"}
        )
        assert organized.status_code == 200 and "整理后的西湖记录" in organized.text
        confirmed = client.post(
            "/api/v1/ai/organize/confirm",
            headers=headers,
            json={
                "note_id": source["id"],
                "title": "整理后的西湖记录",
                "summary": "杭州旅行",
                "content": "# 西湖\n\n天气很好",
            },
        )
        assert confirmed.status_code == 201
        revisions = client.get(f"/api/v1/notes/{source['id']}/revisions", headers=headers).json()
        assert (
            revisions[0]["reason"] == "ai_before_apply"
            and "今天去了杭州" in revisions[0]["content"]
        )

        payload = {"type": "daily", "anchor_date": "2026-07-23"}
        preview = client.post("/api/v1/reports/preview", headers=headers, json=payload).json()
        assert [item["id"] for item in preview["sources"]] == [source["id"]]
        generated = client.post("/api/v1/reports/generate", headers=headers, json=payload)
        assert generated.status_code == 200 and "日报" in generated.text
        saved = client.post(
            "/api/v1/reports/confirm",
            headers=headers,
            json={
                **payload,
                "title": "2026-07-23 日报",
                "content": "# 日报\n\n游览了西湖",
                "source_ids": [source["id"]],
                "overwrite": False,
            },
        )
        assert saved.status_code == 201
        citations = client.get(
            f"/api/v1/reports/{saved.json()['id']}/sources", headers=headers
        ).json()
        assert citations[0]["id"] == source["id"]
        assert (
            client.post(
                "/api/v1/reports/confirm",
                headers=headers,
                json={
                    **payload,
                    "title": "重复",
                    "content": "重复",
                    "source_ids": [source["id"]],
                    "overwrite": False,
                },
            ).status_code
            == 409
        )
        assert (
            client.post(
                "/api/v1/reports/generate",
                headers=headers,
                json={"type": "daily", "anchor_date": "2025-01-01"},
            ).status_code
            == 422
        )

        memory = client.post(
            "/api/v1/memory/chat",
            headers=headers,
            json={"question": "2026年7月23日杭州发生了什么？"},
        )
        assert memory.status_code == 200 and "西湖" in memory.text
        with SessionLocal.begin() as db:
            user = db.query(User).filter(User.username == username).one()
            tenant = db.query(Tenant).filter(Tenant.user_id == user.id).one()
            db.execute(
                __import__("sqlalchemy").text(
                    "SELECT set_config('app.current_tenant_id', :tenant_id, true)"
                ),
                {"tenant_id": str(tenant.id)},
            )
            assistant = (
                db.query(Message)
                .filter(Message.tenant_id == tenant.id, Message.role == "assistant")
                .order_by(Message.id.desc())
                .first()
            )
            assistant_id = assistant.id
        sources = client.get(
            f"/api/v1/memory/messages/{assistant_id}/sources", headers=headers
        ).json()
        assert sources[0]["id"] == source["id"]
        assert (
            client.post(
                "/api/v1/auth/register",
                json={
                    "username": other_username,
                    "email": f"{other_username}@example.invalid",
                    "password": password,
                },
            ).status_code
            == 201
        )
        other_token = client.post(
            "/api/v1/auth/login", json={"username": other_username, "password": password}
        ).json()["token"]
        other_headers = {"Authorization": f"Token {other_token}"}
        assert (
            client.post(
                "/api/v1/notes",
                headers=other_headers,
                json={
                    "type": "normal",
                    "title": "秘密项目",
                    "content": "租户专属词：海王星灯塔",
                    "note_date": "2026-07-23",
                },
            ).status_code
            == 201
        )
        assert (
            client.post(
                "/api/v1/memory/chat", headers=headers, json={"question": "海王星灯塔"}
            ).status_code
            == 404
        )
    finally:
        with SessionLocal.begin() as db:
            users = db.query(User).filter(User.username.in_([username, other_username])).all()
            ids = [user.id for user in users]
            if ids:
                db.query(Tenant).filter(Tenant.user_id.in_(ids)).delete(synchronize_session=False)
                db.query(User).filter(User.id.in_(ids)).delete(synchronize_session=False)
