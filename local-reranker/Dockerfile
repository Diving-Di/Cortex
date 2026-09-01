ARG PYTHON_RUNTIME_IMAGE=python:3.11-slim@sha256:d1053354624536b044162aaab1e418bd000ea35184fb1ae098ab3166b1072e72
FROM ${PYTHON_RUNTIME_IMAGE} AS prepared

ARG RERANK_MODEL_REVISION=e099d4b9cdbd291b1569d416f19aaa6523570bc3
ARG TORCH_INDEX_URL=https://download.pytorch.org/whl/cu130

ENV HF_HUB_OFFLINE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    RERANK_MODEL_PATH=/models/reranker

WORKDIR /app
COPY requirements.txt .
RUN apt-get update && apt-get upgrade -y && \
    apt-get install -y --no-install-recommends git git-lfs && \
    rm -rf /var/lib/apt/lists/*
RUN python -m pip install --no-cache-dir --upgrade pip setuptools wheel && \
    pip install --no-cache-dir \
      --index-url "${TORCH_INDEX_URL}" torch==2.13.0 && \
    pip install --no-cache-dir -r requirements.txt
RUN GIT_LFS_SKIP_SMUDGE=1 git clone https://www.modelscope.cn/BAAI/bge-reranker-v2-m3.git /models/reranker && \
    GIT_LFS_SKIP_SMUDGE=1 git -C /models/reranker checkout "${RERANK_MODEL_REVISION}" && \
    for attempt in 1 2 3; do \
      git -C /models/reranker lfs pull && break; \
      if [ "${attempt}" = 3 ]; then exit 1; fi; \
      sleep $((attempt * 10)); \
    done && \
    rm -rf /models/reranker/.git && \
    useradd --create-home --uid 10001 reranker
RUN python -m pip uninstall -y pip

COPY app.py .
RUN chown -R reranker:reranker /app && chmod -R a-w /models

FROM scratch
COPY --from=prepared / /

ENV HF_HUB_OFFLINE=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    RERANK_MODEL_PATH=/models/reranker
WORKDIR /app
USER 10001:10001

EXPOSE 8080
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]
