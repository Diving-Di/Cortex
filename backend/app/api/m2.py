from __future__ import annotations
import asyncio, json, time
from datetime import date, datetime, timezone
from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, ConfigDict, Field
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session
from ..ai.client import OpenAICompatibleClient
from ..core.config import get_settings
from ..core.database import get_db
from ..core.exceptions import AppError
from ..core.tenant import TenantContext, get_tenant_context
from ..models import (
    AiUsageRecord,
    Conversation,
    Message,
    MessageSource,
    Note,
    NoteRevision,
    ReportSource,
)
from ..schemas import NoteCreate
from ..services.m2_service import memory_candidates, report_sources, snippet
from ..services.note_service import create_note, get_note

router = APIRouter(prefix="/api/v1", tags=["m2-ai"])


class OrganizeIn(BaseModel):
    model_config = ConfigDict(extra="forbid")
    content: str = Field(min_length=1, max_length=50000)
    style: str = "structured"


class OrganizeConfirm(BaseModel):
    model_config = ConfigDict(extra="forbid")
    title: str
    content: str
    summary: str | None = None
    note_id: int | None = None


class ReportIn(BaseModel):
    model_config = ConfigDict(extra="forbid")
    type: str
    anchor_date: date


class ReportConfirm(ReportIn):
    title: str
    content: str
    source_ids: list[int]
    overwrite: bool = False


class MemoryIn(BaseModel):
    model_config = ConfigDict(extra="forbid")
    question: str = Field(min_length=1, max_length=5000)
    conversation_id: int | None = None


def _client_and_model() -> tuple[OpenAICompatibleClient, str]:
    config = get_settings().ai
    if not config.get("api_key"):
        raise AppError("AI_NOT_CONFIGURED", "AI 未配置，笔记功能仍可正常使用", 503)
    return OpenAICompatibleClient(config["base_url"], config["api_key"]), config["model"]


def _sse(
    request: Request,
    db: Session,
    context: TenantContext,
    request_type: str,
    prompt: str,
    after=None,
) -> StreamingResponse:
    client, model = _client_and_model()
    started = time.perf_counter()

    async def events():
        chunks: list[str] = []
        status = "success"
        error = None
        try:
            async for chunk in client.stream_chat(model, [{"role": "user", "content": prompt}]):
                if await request.is_disconnected():
                    status = "cancelled"
                    break
                chunks.append(chunk)
                yield f"data: {json.dumps({'content':chunk}, ensure_ascii=False)}\n\n"
            if status == "success" and after:
                after("".join(chunks))
            yield "data: [DONE]\n\n"
        except asyncio.CancelledError:
            status = "cancelled"
            error = "CLIENT_DISCONNECTED"
            raise
        except Exception as exc:
            status = "error"
            error = type(exc).__name__
            yield f"event: error\ndata: {json.dumps({'code':'AI_REQUEST_FAILED'})}\n\n"
        finally:
            db.add(
                AiUsageRecord(
                    tenant_id=context.tenant_id,
                    user_id=context.user_id,
                    request_type=request_type,
                    model=model,
                    input_tokens=max(1, len(prompt) // 4),
                    output_tokens=sum(map(len, chunks)) // 4,
                    duration_ms=int((time.perf_counter() - started) * 1000),
                    status=status,
                    error_code=error,
                )
            )
            db.commit()

    return StreamingResponse(
        events(),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


@router.post("/ai/organize")
def organize(
    payload: OrganizeIn,
    request: Request,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    prompt = (
        '你是中文笔记整理助手。不得添加原文没有的事实。请仅输出 JSON：{"title":"标题","summary":"摘要","content":"Markdown 正文"}。整理风格：%s。原始记录：\n%s'
        % (payload.style, payload.content)
    )
    return _sse(request, db, context, "organize", prompt)


@router.post("/ai/organize/confirm", status_code=201)
def confirm_organize(
    payload: OrganizeConfirm,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> dict:
    if payload.note_id:
        note = get_note(db, context, payload.note_id)
        db.add(
            NoteRevision(
                tenant_id=context.tenant_id,
                note_id=note.id,
                created_by=context.user_id,
                content=note.content,
                reason="ai_before_apply",
            )
        )
        note.title = payload.title.strip()
        note.content = payload.content
        note.summary = payload.summary
        note.word_count = len("".join(payload.content.split()))
        note.updated_by = context.user_id
        note.updated_at = datetime.now(timezone.utc)
    else:
        note = create_note(
            db,
            context,
            NoteCreate(title=payload.title, content=payload.content, summary=payload.summary),
        )
    db.flush()
    return {"id": note.id, "title": note.title, "content": note.content}


@router.post("/reports/preview")
def preview_report(
    payload: ReportIn,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> dict:
    start, end, notes = report_sources(db, context, payload.type, payload.anchor_date)
    return {
        "start_date": start,
        "end_date": end,
        "sources": [
            {
                "id": n.id,
                "title": n.title,
                "note_date": n.note_date,
                "snippet": snippet(n.content, 160),
            }
            for n in notes
        ],
    }


@router.post("/reports/generate")
def generate_report(
    payload: ReportIn,
    request: Request,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    start, end, notes = report_sources(db, context, payload.type, payload.anchor_date)
    if not notes:
        raise AppError("REPORT_NO_SOURCES", "所选周期没有来源笔记，已阻止生成虚构报告", 422)
    material = "\n\n".join(
        f"[来源 #{n.id} {n.note_date} {n.title}]\n{snippet(n.content,2000)}" for n in notes
    )
    prompt = f"仅依据以下来源撰写 {payload.type} Markdown 报告。每个事实使用 [#{'{'}笔记ID{'}'}] 引用，不得虚构。周期 {start} 至 {end}。\n\n{material}"
    return _sse(request, db, context, "report", prompt)


@router.post("/reports/confirm", status_code=201)
def confirm_report(
    payload: ReportConfirm,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> dict:
    start, _, sources = report_sources(db, context, payload.type, payload.anchor_date)
    allowed = {n.id for n in sources}
    if not payload.source_ids or not set(payload.source_ids).issubset(allowed):
        raise AppError("INVALID_REPORT_SOURCES", "报告来源为空或不属于所选周期", 422)
    note = (
        db.query(Note)
        .filter(
            Note.tenant_id == context.tenant_id,
            Note.type == payload.type,
            Note.note_date == start,
            Note.deleted_at.is_(None),
        )
        .one_or_none()
    )
    if note and not payload.overwrite:
        raise AppError("REPORT_EXISTS", "该周期报告已存在，请明确选择覆盖", 409)
    if note:
        db.add(
            NoteRevision(
                tenant_id=context.tenant_id,
                note_id=note.id,
                created_by=context.user_id,
                content=note.content,
                reason="report_before_overwrite",
            )
        )
        note.title = payload.title
        note.content = payload.content
        note.word_count = len("".join(payload.content.split()))
        note.updated_at = datetime.now(timezone.utc)
        db.query(ReportSource).filter(
            ReportSource.tenant_id == context.tenant_id, ReportSource.report_note_id == note.id
        ).delete()
    else:
        try:
            note = create_note(
                db,
                context,
                NoteCreate(
                    type=payload.type, title=payload.title, content=payload.content, note_date=start
                ),
            )
        except IntegrityError as exc:
            raise AppError("REPORT_EXISTS", "该周期报告已存在", 409) from exc
    db.flush()
    for rank, note_id in enumerate(payload.source_ids, 1):
        db.add(
            ReportSource(
                tenant_id=context.tenant_id,
                report_note_id=note.id,
                source_note_id=note_id,
                rank=rank,
            )
        )
    return {"id": note.id, "source_ids": payload.source_ids}


@router.get("/reports/{note_id}/sources")
def get_report_sources(
    note_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> list[dict]:
    get_note(db, context, note_id)
    rows = (
        db.query(ReportSource, Note)
        .join(Note, Note.id == ReportSource.source_note_id)
        .filter(
            ReportSource.tenant_id == context.tenant_id,
            ReportSource.report_note_id == note_id,
            Note.tenant_id == context.tenant_id,
        )
        .order_by(ReportSource.rank)
        .all()
    )
    return [
        {"id": n.id, "title": n.title, "note_date": n.note_date, "snippet": snippet(n.content, 160)}
        for _, n in rows
    ]


@router.post("/memory/chat")
def memory_chat(
    payload: MemoryIn,
    request: Request,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
):
    notes = memory_candidates(db, context, payload.question)
    if not notes:
        raise AppError("MEMORY_NO_EVIDENCE", "没有找到足够的笔记记录", 404)
    material = "\n\n".join(
        f"[#{n.id} {n.note_date or ''} {n.title}] {snippet(n.content,1200)}" for n in notes
    )
    prompt = f"仅依据给定笔记回答问题，引用格式为 [#笔记ID]；证据不足就明确说没有找到记录。\n问题：{payload.question}\n笔记：\n{material}"

    def save(answer: str) -> None:
        conversation = None
        if payload.conversation_id:
            conversation = (
                db.query(Conversation)
                .filter(
                    Conversation.id == payload.conversation_id,
                    Conversation.tenant_id == context.tenant_id,
                )
                .one_or_none()
            )
        if conversation is None:
            conversation = Conversation(
                tenant_id=context.tenant_id, user_id=context.user_id, title=payload.question[:80]
            )
            db.add(conversation)
            db.flush()
        db.add(
            Message(
                tenant_id=context.tenant_id,
                conversation_id=conversation.id,
                role="user",
                content=payload.question,
            )
        )
        assistant = Message(
            tenant_id=context.tenant_id,
            conversation_id=conversation.id,
            role="assistant",
            content=answer,
        )
        db.add(assistant)
        db.flush()
        for rank, n in enumerate(notes, 1):
            db.add(
                MessageSource(
                    tenant_id=context.tenant_id,
                    message_id=assistant.id,
                    note_id=n.id,
                    snippet=snippet(n.content),
                    relevance=len(notes) - rank + 1,
                    rank=rank,
                )
            )

    return _sse(request, db, context, "memory", prompt, save)


@router.get("/memory/messages/{message_id}/sources")
def memory_sources(
    message_id: int,
    db: Session = Depends(get_db),
    context: TenantContext = Depends(get_tenant_context),
) -> list[dict]:
    rows = (
        db.query(MessageSource, Note)
        .join(Note, Note.id == MessageSource.note_id)
        .filter(
            MessageSource.tenant_id == context.tenant_id,
            MessageSource.message_id == message_id,
            Note.tenant_id == context.tenant_id,
        )
        .order_by(MessageSource.rank)
        .all()
    )
    return [{"id": n.id, "title": n.title, "snippet": s.snippet, "rank": s.rank} for s, n in rows]
