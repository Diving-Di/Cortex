FROM python:3.11-slim@sha256:db3ff2e1800a8581e2c48a27c3995339d47bdf046da21c7627accd3d51053a93

ARG MODEL_REVISION=3c386e2c2c38680eaa38977bbf69c60d72fbf24f

ENV HF_HUB_OFFLINE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    EMBEDDING_MODEL_PATH=/models/embedding

WORKDIR /app
COPY requirements.txt .
RUN apt-get update && apt-get install -y --no-install-recommends git git-lfs \
    && pip install --no-cache-dir --index-url https://download.pytorch.org/whl/cpu torch==2.13.0 \
    && pip install --no-cache-dir -r requirements.txt \
    && git clone https://www.modelscope.cn/iic/nlp_gte_sentence-embedding_chinese-small.git /models/embedding \
    && git -C /models/embedding checkout "${MODEL_REVISION}" \
    && rm -rf /models/embedding/.git /var/lib/apt/lists/* \
    && useradd --create-home --uid 10001 embedding \
    && chmod -R a-w /models

COPY app.py .
RUN chown -R embedding:embedding /app
USER 10001:10001
EXPOSE 4000
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "4000", "--no-access-log"]
