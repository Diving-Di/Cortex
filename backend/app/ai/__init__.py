from ..core.config import get_settings

def generate_reply(messages: list[dict[str, str]]) -> str:
    """Legacy endpoint remains local/non-blocking with respect to external AI."""
    if not get_settings().ai.get("api_key"):
        return f"（本地模式）我收到了：{messages[-1]['content']}"
    return "请使用 /api/v1/ai/stream 获取异步流式回复。"
