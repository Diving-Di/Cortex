"""DeepSeek / OpenAI compatible chat client.

If an api key is configured (via config.json or env), it calls the configured
chat-completions endpoint. Otherwise it falls back to a simple local echo reply
so the app stays runnable without any credentials.
"""
from __future__ import annotations

import json
import urllib.error
import urllib.request
from typing import Dict, List

from .config import get_config


def _local_fallback(messages: List[Dict[str, str]]) -> str:
    last_user = next(
        (m["content"] for m in reversed(messages) if m.get("role") == "user"),
        "",
    )
    return (
        "（本地模拟回复，未配置 AI API Key）\n"
        f"我收到了你的消息：{last_user}\n"
        "在 backend/config.json 中填入 DeepSeek 的 api_key 即可获得真实回复。"
    )


def generate_reply(history: List[Dict[str, str]]) -> str:
    """`history` is a list of {role, content} dicts in chronological order."""
    config = get_config()
    ai = config["ai"]
    api_key = (ai.get("api_key") or "").strip()

    system_prompt = ai.get("system_prompt") or ""
    messages: List[Dict[str, str]] = []
    if system_prompt:
        messages.append({"role": "system", "content": system_prompt})
    messages.extend(history)

    if not api_key:
        return _local_fallback(messages)

    url = ai["base_url"].rstrip("/") + "/chat/completions"
    payload = {
        "model": ai.get("model", "deepseek-chat"),
        "messages": messages,
        "stream": False,
    }
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            body = json.loads(resp.read().decode("utf-8"))
        return body["choices"][0]["message"]["content"]
    except urllib.error.HTTPError as exc:  # pragma: no cover - network dependent
        detail = exc.read().decode("utf-8", errors="ignore")
        return f"（AI 接口调用失败：HTTP {exc.code}）{detail[:200]}"
    except Exception as exc:  # pragma: no cover - network dependent
        return f"（AI 接口调用失败：{exc}）"
