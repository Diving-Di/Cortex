from datetime import date, datetime
from typing import Literal, Optional
from pydantic import BaseModel, ConfigDict


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
    messages: list[MessageOut] = []


class ChatIn(BaseModel):
    content: str
    conversation_id: Optional[int] = None


class ChatOut(BaseModel):
    conversation_id: int
    title: str
    user_message: MessageOut
    assistant_message: MessageOut


class DiaryOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: int
    image: Optional[str] = None
    content: str
    created_at: datetime


NoteType = Literal["normal", "daily", "weekly", "monthly"]


class NoteCreate(BaseModel):
    model_config = ConfigDict(extra="forbid")
    type: NoteType = "normal"
    title: str
    content: str = ""
    note_date: Optional[date] = None
    summary: Optional[str] = None


class NoteUpdate(BaseModel):
    model_config = ConfigDict(extra="forbid")
    title: Optional[str] = None
    content: Optional[str] = None
    note_date: Optional[date] = None
    summary: Optional[str] = None
    expected_updated_at: Optional[datetime] = None


class NoteOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: int
    type: NoteType
    title: str
    content: str
    note_date: Optional[date]
    summary: Optional[str]
    word_count: int
    created_at: datetime
    updated_at: datetime


class NotePage(BaseModel):
    items: list[NoteOut]
    page: int
    page_size: int
    total: int


class RevisionOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: int
    note_id: int
    content: str
    reason: str
    created_at: datetime


class TenantOut(BaseModel):
    id: str
    name: str
    status: str
    note_quota: int
    note_count: int
    attachment_quota_bytes: int
    ai_token_quota: int
    ai_tokens_used: int


class TenantUpdate(BaseModel):
    model_config = ConfigDict(extra="forbid")
    name: str


class TagCreate(BaseModel):
    model_config = ConfigDict(extra="forbid")
    name: str
    color: Optional[str] = None


class TagOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: int
    name: str
    color: Optional[str]


class TagAssignment(BaseModel):
    model_config = ConfigDict(extra="forbid")
    tag_ids: list[int]


class AttachmentOut(BaseModel):
    model_config = ConfigDict(from_attributes=True)
    id: int
    note_id: int
    original_name: str
    mime_type: str
    size: int
    sha256: str
    created_at: datetime


class SearchResult(BaseModel):
    id: int
    title: str
    snippet: str
    type: NoteType
    note_date: Optional[date]
    updated_at: datetime


class SearchPage(BaseModel):
    items: list[SearchResult]
    total: int
