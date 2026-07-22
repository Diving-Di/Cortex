from __future__ import annotations
import asyncio, json
from collections.abc import AsyncIterator
import httpx


class OpenAICompatibleClient:
    def __init__(
        self,
        base_url: str,
        api_key: str,
        timeout: float = 60.0,
        transport: httpx.AsyncBaseTransport | None = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.timeout = timeout
        self.transport = transport

    async def stream_chat(self, model: str, messages: list[dict[str, str]]) -> AsyncIterator[str]:
        if not self.api_key:
            raise RuntimeError("AI_NOT_CONFIGURED")
        headers = {"Authorization": f"Bearer {self.api_key}"}
        payload = {"model": model, "messages": messages, "stream": True}
        for attempt in range(2):
            try:
                async with httpx.AsyncClient(
                    timeout=httpx.Timeout(self.timeout, connect=10.0), transport=self.transport
                ) as client:
                    async with client.stream(
                        "POST", f"{self.base_url}/chat/completions", headers=headers, json=payload
                    ) as response:
                        response.raise_for_status()
                        async for line in response.aiter_lines():
                            if not line.startswith("data:"):
                                continue
                            raw = line[5:].strip()
                            if raw == "[DONE]":
                                return
                            data = json.loads(raw)
                            content = data.get("choices", [{}])[0].get("delta", {}).get("content")
                            if content:
                                yield content
                        return
            except (httpx.ConnectError, httpx.ConnectTimeout, httpx.ReadTimeout):
                if attempt:
                    raise
                await asyncio.sleep(0.25)
