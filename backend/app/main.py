from __future__ import annotations

import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from .config import get_config
from .database import init_db
from .routers import auth, chat, diary

config = get_config()

app = FastAPI(
    title="AI Chat & Diary API",
    description="FastAPI + SQLAlchemy backend providing AI chat and light diary features.",
    version="2.0.0",
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

app.include_router(auth.router)
app.include_router(chat.router)
app.include_router(diary.router)

os.makedirs(config["media_dir"], exist_ok=True)
app.mount("/media", StaticFiles(directory=config["media_dir"]), name="media")


@app.on_event("startup")
def on_startup() -> None:
    init_db()


@app.get("/healthz", tags=["system"])
def healthz() -> dict:
    return {"status": "ok"}
