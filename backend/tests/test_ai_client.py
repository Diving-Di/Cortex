import asyncio
import httpx
from app.ai.client import OpenAICompatibleClient

def test_openai_compatible_sse_and_limited_retry() -> None:
    attempts = 0
    async def handler(request: httpx.Request) -> httpx.Response:
        nonlocal attempts
        attempts += 1
        assert request.headers['authorization'] == 'Bearer secret'
        if attempts == 1: raise httpx.ConnectError('temporary', request=request)
        body = 'data: {"choices":[{"delta":{"content":"你"}}]}\n\ndata: {"choices":[{"delta":{"content":"好"}}]}\n\ndata: [DONE]\n\n'
        return httpx.Response(200, text=body, headers={'content-type':'text/event-stream'})
    async def collect() -> list[str]:
        client=OpenAICompatibleClient('https://example.test/v1','secret',transport=httpx.MockTransport(handler))
        return [chunk async for chunk in client.stream_chat('model',[{'role':'user','content':'hi'}])]
    assert asyncio.run(collect()) == ['你','好']
    assert attempts == 2
