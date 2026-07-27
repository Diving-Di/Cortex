FROM python:3.11-slim@sha256:db3ff2e1800a8581e2c48a27c3995339d47bdf046da21c7627accd3d51053a93

ARG RERANK_MODEL=Qwen/Qwen3-Reranker-0.6B
ARG RERANK_MODEL_REVISION=e61197ed45024b0ed8a2d74b80b4d909f1255473

ENV HF_HOME=/models \
    HF_HUB_OFFLINE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    RERANK_MODEL=${RERANK_MODEL} \
    RERANK_MODEL_REVISION=${RERANK_MODEL_REVISION}

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir \
      --index-url https://download.pytorch.org/whl/cpu torch==2.7.1 && \
    pip install --no-cache-dir -r requirements.txt && \
    HF_HUB_OFFLINE=0 python -c \
      "from huggingface_hub import snapshot_download; snapshot_download('${RERANK_MODEL}', revision='${RERANK_MODEL_REVISION}')" && \
    useradd --create-home --uid 10001 reranker

COPY app.py .
RUN chown -R reranker:reranker /app && chmod -R a-w /models

USER 10001:10001

EXPOSE 8080
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]
