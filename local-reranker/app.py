import math
import os
import threading
from contextlib import asynccontextmanager
from typing import Annotated

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, ConfigDict, Field
from sentence_transformers import CrossEncoder
import torch


MODEL_ID = "BAAI/bge-reranker-v2-m3"
MODEL_PATH = os.getenv("RERANK_MODEL_PATH", "/models/reranker")
DEVICE = os.getenv("RERANK_DEVICE", "cpu").strip().lower()
MAX_DOCUMENTS = int(os.getenv("RERANK_MAX_DOCUMENTS", "20"))
MAX_LENGTH = int(os.getenv("RERANK_MAX_LENGTH", "2048"))
DocumentText = Annotated[str, Field(min_length=1, max_length=131_072)]


class RerankRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    model: str
    query: str = Field(min_length=1, max_length=16_384)
    documents: list[DocumentText] = Field(min_length=1, max_length=MAX_DOCUMENTS)


class RerankResult(BaseModel):
    index: int
    relevance_score: float


class RerankResponse(BaseModel):
    results: list[RerankResult]


class RecipeReranker:
    def __init__(self) -> None:
        if DEVICE == "cuda" and not torch.cuda.is_available():
            raise RuntimeError("RERANK_DEVICE=cuda but CUDA is unavailable")
        self.model = CrossEncoder(MODEL_PATH, max_length=MAX_LENGTH, device=DEVICE)
        self.lock = threading.Lock()

    def score(self, query: str, documents: list[str]) -> list[float]:
        with self.lock:
            values = self.model.predict([(query, document) for document in documents])
        return [float(value) for value in values]


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.reranker = RecipeReranker()
    app.state.reranker.score("健康检查", ["健康检查"])
    yield


app = FastAPI(title="Diary Listener recipe reranker", lifespan=lifespan)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "model": MODEL_ID, "device": DEVICE}


@app.post("/rerank", response_model=RerankResponse)
def rerank(request: RerankRequest) -> RerankResponse:
    if request.model != MODEL_ID:
        raise HTTPException(status_code=400, detail="unsupported model")
    scores = app.state.reranker.score(request.query, request.documents)
    if len(scores) != len(request.documents) or any(not math.isfinite(score) for score in scores):
        raise HTTPException(status_code=502, detail="invalid model output")
    order = sorted(range(len(scores)), key=lambda index: scores[index], reverse=True)
    return RerankResponse(
        results=[
            RerankResult(index=index, relevance_score=scores[index])
            for index in order
        ]
    )
