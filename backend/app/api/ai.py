from __future__ import annotations
import asyncio, json, time
from fastapi import APIRouter, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, ConfigDict
from sqlalchemy.orm import Session
from ..ai.client import OpenAICompatibleClient
from ..core.config import get_settings
from ..core.exceptions import AppError
from ..core.database import get_db
from ..core.tenant import TenantContext, get_tenant_context
from ..models import AiProvider, AiUsageRecord

router = APIRouter(prefix="/api/v1", tags=["ai"])
class ChatRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    prompt: str
    model: str | None = None
class ProviderIn(BaseModel):
    model_config = ConfigDict(extra="forbid")
    display_name: str
    base_url: str
    default_model: str
    capabilities: str = "chat,stream"

@router.get("/settings/ai")
def ai_settings(context: TenantContext = Depends(get_tenant_context)) -> dict:
    config = get_settings().ai
    return {"configured": bool(config.get("api_key")), "base_url": config.get("base_url"), "model": config.get("model"), "api_key": None}

@router.post("/ai/providers")
def configure_provider(payload: ProviderIn, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> dict:
    provider = db.query(AiProvider).filter(AiProvider.tenant_id == context.tenant_id).first()
    if provider is None: provider = AiProvider(tenant_id=context.tenant_id, **payload.model_dump()); db.add(provider)
    else:
        for key, value in payload.model_dump().items(): setattr(provider, key, value)
    db.flush(); return {"id": provider.id, "display_name": provider.display_name, "base_url": provider.base_url, "default_model": provider.default_model, "capabilities": provider.capabilities}

@router.post("/ai/stream")
def stream_ai(payload: ChatRequest, request: Request, db: Session = Depends(get_db), context: TenantContext = Depends(get_tenant_context)) -> StreamingResponse:
    config = get_settings().ai
    if not config.get("api_key"): raise AppError("AI_NOT_CONFIGURED", "AI 未配置，笔记功能仍可正常使用", 503)
    model = payload.model or config["model"]; client = OpenAICompatibleClient(config["base_url"], config["api_key"]); started = time.perf_counter()
    async def events():
        output = 0; status = "success"; error = None
        try:
            async for chunk in client.stream_chat(model, [{"role": "user", "content": payload.prompt}]):
                if await request.is_disconnected(): status = "cancelled"; break
                output += len(chunk); yield f"data: {json.dumps({'content': chunk}, ensure_ascii=False)}\n\n"
            yield "data: [DONE]\n\n"
        except asyncio.CancelledError:
            status = "cancelled"; error = "CLIENT_DISCONNECTED"; raise
        except Exception as exc:
            status = "error"; error = type(exc).__name__; yield f"event: error\ndata: {json.dumps({'code': 'AI_REQUEST_FAILED'})}\n\n"
        finally:
            db.add(AiUsageRecord(tenant_id=context.tenant_id, user_id=context.user_id, request_type="chat", model=model, input_tokens=max(1, len(payload.prompt)//4), output_tokens=max(0, output//4), duration_ms=int((time.perf_counter()-started)*1000), status=status, error_code=error)); db.commit()
    return StreamingResponse(events(), media_type="text/event-stream", headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"})
