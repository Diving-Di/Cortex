"""Pydantic schemas (request / response models)."""
from __future__ import annotations

from datetime import datetime
from typing import List, Optional

from pydantic import BaseModel, ConfigDict


# ---- auth ----
class RegisterIn(BaseModel):
    username: str
    email: str
    password: str


class LoginIn(BaseModel):
    username: str
    password: str


class LoginOut(BaseModel):
    token: str
    username: str


# ---- chat ----
class MessageOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    role: str
    content: str
    created_at: datetime


class ConversationOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    title: str
    created_at: datetime
    updated_at: datetime


class ConversationDetailOut(ConversationOut):
    messages: List[MessageOut] = []


class ChatIn(BaseModel):
    content: str
    conversation_id: Optional[int] = None


class ChatOut(BaseModel):
    conversation_id: int
    title: str
    user_message: MessageOut
    assistant_message: MessageOut


# ---- diary ----
class DiaryOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    image: Optional[str] = None
    content: str
    created_at: datetime
