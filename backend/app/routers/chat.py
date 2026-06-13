from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Response, status
from sqlalchemy import func
from sqlalchemy.orm import Session

from ..ai import generate_reply
from ..database import get_db
from ..models import Conversation, Message, User
from ..schemas import (
    ChatIn,
    ChatOut,
    ConversationDetailOut,
    ConversationOut,
    MessageOut,
)
from ..security import get_current_user

router = APIRouter(prefix="/api/chat", tags=["chat"])

_MAX_HISTORY = 20  # number of recent messages sent to the AI


def _get_owned_conversation(db: Session, conversation_id: int, user: User) -> Conversation:
    conv = (
        db.query(Conversation)
        .filter(Conversation.id == conversation_id, Conversation.user_id == user.id)
        .first()
    )
    if not conv:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="对话不存在")
    return conv


@router.get("/conversations/", response_model=list[ConversationOut])
def list_conversations(
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> list[Conversation]:
    return (
        db.query(Conversation)
        .filter(Conversation.user_id == user.id)
        .order_by(Conversation.updated_at.desc())
        .all()
    )


@router.get("/conversations/{conversation_id}/", response_model=ConversationDetailOut)
def get_conversation(
    conversation_id: int,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> Conversation:
    return _get_owned_conversation(db, conversation_id, user)


@router.delete("/conversations/{conversation_id}/", status_code=status.HTTP_204_NO_CONTENT)
def delete_conversation(
    conversation_id: int,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> Response:
    conv = _get_owned_conversation(db, conversation_id, user)
    db.delete(conv)
    db.commit()
    return Response(status_code=status.HTTP_204_NO_CONTENT)


@router.post("/", response_model=ChatOut)
def send_message(
    payload: ChatIn,
    db: Session = Depends(get_db),
    user: User = Depends(get_current_user),
) -> ChatOut:
    content = payload.content.strip()
    if not content:
        raise HTTPException(status_code=status.HTTP_400_BAD_REQUEST, detail="消息不能为空")

    if payload.conversation_id:
        conv = _get_owned_conversation(db, payload.conversation_id, user)
    else:
        title = content[:20] + ("…" if len(content) > 20 else "")
        conv = Conversation(user_id=user.id, title=title or "新的对话")
        db.add(conv)
        db.flush()

    user_msg = Message(conversation_id=conv.id, role="user", content=content)
    db.add(user_msg)
    db.flush()

    history = [
        {"role": m.role, "content": m.content}
        for m in conv.messages[-_MAX_HISTORY:]
    ]
    reply = generate_reply(history)

    assistant_msg = Message(conversation_id=conv.id, role="assistant", content=reply)
    db.add(assistant_msg)
    conv.updated_at = func.now()
    db.commit()
    db.refresh(user_msg)
    db.refresh(assistant_msg)

    return ChatOut(
        conversation_id=conv.id,
        title=conv.title,
        user_message=MessageOut.model_validate(user_msg),
        assistant_message=MessageOut.model_validate(assistant_msg),
    )
