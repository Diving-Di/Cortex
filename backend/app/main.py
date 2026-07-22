from __future__ import annotations

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from .api.system import router as system_router
from .api.notes import router as notes_router
from .api.tenant import router as tenant_router
from .core.config import get_settings
from .core.exceptions import install_exception_handlers
from .core.logging import configure_logging
from .routers import auth, chat, diary

settings = get_settings()
configure_logging(settings.log_level)

app = FastAPI(
    title="Diary Listener API",
    description="Local-first, tenant-isolated notes API.",
    version="3.0.0",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=list(settings.cors_origins),
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(auth.router)
app.include_router(chat.router)
app.include_router(diary.router)
app.include_router(system_router)
app.include_router(tenant_router)
app.include_router(notes_router)
install_exception_handlers(app)
