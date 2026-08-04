import os
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, ConfigDict, Field
from sentence_transformers import SentenceTransformer


MODEL_ID = "iic/nlp_gte_sentence-embedding_chinese-small"
MODEL_PATH = os.getenv("EMBEDDING_MODEL_PATH", "/models/embedding")
EXPECTED_DIMENSIONS = 512


class EmbeddingRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    input: str | list[str]
    model: str | None = None
    encoding_format: str | None = None
    dimensions: int | None = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.model = SentenceTransformer(MODEL_PATH, device="cpu")
    probe = app.state.model.encode(["健康检查"], normalize_embeddings=True)
    if probe.shape != (1, EXPECTED_DIMENSIONS):
        raise RuntimeError(f"embedding dimensions must be {EXPECTED_DIMENSIONS}")
    yield


app = FastAPI(title="Diary Listener recipe embedding", lifespan=lifespan)


@app.get("/healthz")
def health() -> dict[str, object]:
    return {"status": "ok", "model": MODEL_ID, "dimensions": EXPECTED_DIMENSIONS}


@app.post("/v1/embeddings")
def embeddings(request: EmbeddingRequest) -> dict[str, object]:
    if request.model not in (None, MODEL_ID):
        raise HTTPException(status_code=400, detail="unsupported model")
    if request.encoding_format not in (None, "float"):
        raise HTTPException(status_code=400, detail="unsupported encoding format")
    if request.dimensions not in (None, EXPECTED_DIMENSIONS):
        raise HTTPException(status_code=400, detail="unsupported dimensions")
    values = [request.input] if isinstance(request.input, str) else request.input
    if not values or len(values) > 64 or any(not value.strip() for value in values):
        raise HTTPException(status_code=400, detail="invalid input")
    vectors = app.state.model.encode(values, normalize_embeddings=True)
    if vectors.shape[1] != EXPECTED_DIMENSIONS:
        raise HTTPException(status_code=502, detail="invalid model output")
    return {
        "object": "list",
        "data": [
            {"object": "embedding", "embedding": vector.tolist(), "index": index}
            for index, vector in enumerate(vectors)
        ],
        "model": MODEL_ID,
        "usage": {"prompt_tokens": 0, "total_tokens": 0},
    }
