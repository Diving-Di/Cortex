FROM python:3.11-slim@sha256:db3ff2e1800a8581e2c48a27c3995339d47bdf046da21c7627accd3d51053a93

ARG RERANK_MODEL_REVISION=e099d4b9cdbd291b1569d416f19aaa6523570bc3

ENV HF_HUB_OFFLINE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    RERANK_MODEL_PATH=/models/reranker

WORKDIR /app
COPY requirements.txt .
RUN apt-get update && apt-get install -y --no-install-recommends git git-lfs && \
    pip install --no-cache-dir \
      --index-url https://download.pytorch.org/whl/cu130 torch==2.13.0 && \
    pip install --no-cache-dir -r requirements.txt && \
    git clone https://www.modelscope.cn/BAAI/bge-reranker-v2-m3.git /models/reranker && \
    git -C /models/reranker checkout "${RERANK_MODEL_REVISION}" && \
    rm -rf /models/reranker/.git /var/lib/apt/lists/* && \
    useradd --create-home --uid 10001 reranker

COPY app.py .
RUN chown -R reranker:reranker /app && chmod -R a-w /models

USER 10001:10001

EXPOSE 8080
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]
