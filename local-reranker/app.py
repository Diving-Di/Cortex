import math
import os
import threading
from contextlib import asynccontextmanager
from typing import Annotated

import torch
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, ConfigDict, Field
from transformers import AutoModelForCausalLM, AutoTokenizer


MODEL_ID = os.getenv("RERANK_MODEL", "Qwen/Qwen3-Reranker-0.6B")
MODEL_REVISION = os.getenv(
    "RERANK_MODEL_REVISION",
    "e61197ed45024b0ed8a2d74b80b4d909f1255473",
)
MAX_DOCUMENTS = int(os.getenv("RERANK_MAX_DOCUMENTS", "20"))
MAX_LENGTH = int(os.getenv("RERANK_MAX_LENGTH", "2048"))
BATCH_SIZE = int(os.getenv("RERANK_BATCH_SIZE", "4"))
CPU_THREADS = int(os.getenv("RERANK_CPU_THREADS", "4"))
DocumentText = Annotated[str, Field(min_length=1, max_length=131_072)]
TASK = (
    "Given a personal knowledge-base search query, retrieve passages that provide "
    "direct evidence for answering the query."
)
PREFIX = (
    "<|im_start|>system\nJudge whether the Document meets the requirements based on "
    'the Query and the Instruct provided. Note that the answer can only be "yes" or '
    '"no".<|im_end|>\n<|im_start|>user\n'
)
SUFFIX = "<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n"


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


class QwenReranker:
    def __init__(self) -> None:
        device = "cuda" if torch.cuda.is_available() else "cpu"
        dtype = torch.float16 if device == "cuda" else torch.float32
        if device == "cpu":
            torch.set_num_threads(CPU_THREADS)
            torch.set_num_interop_threads(1)
        offline = os.getenv("HF_HUB_OFFLINE") == "1"
        self.tokenizer = AutoTokenizer.from_pretrained(
            MODEL_ID,
            revision=MODEL_REVISION,
            trust_remote_code=False,
            local_files_only=offline,
            padding_side="left",
        )
        self.model = AutoModelForCausalLM.from_pretrained(
            MODEL_ID,
            revision=MODEL_REVISION,
            trust_remote_code=False,
            local_files_only=offline,
            torch_dtype=dtype,
        ).to(device).eval()
        self.false_token_id = self.tokenizer.convert_tokens_to_ids("no")
        self.true_token_id = self.tokenizer.convert_tokens_to_ids("yes")
        if self.false_token_id == self.tokenizer.unk_token_id or self.true_token_id == self.tokenizer.unk_token_id:
            raise RuntimeError("Qwen3 reranker yes/no tokens are unavailable")
        self.prefix_tokens = self.tokenizer.encode(PREFIX, add_special_tokens=False)
        self.suffix_tokens = self.tokenizer.encode(SUFFIX, add_special_tokens=False)
        if MAX_LENGTH <= len(self.prefix_tokens) + len(self.suffix_tokens):
            raise RuntimeError("RERANK_MAX_LENGTH is too small")
        self.device = device
        self.lock = threading.Lock()

    def _score_batch(self, query: str, documents: list[str]) -> list[float]:
        pairs = [
            f"<Instruct>: {TASK}\n<Query>: {query}\n<Document>: {document}"
            for document in documents
        ]
        inputs = self.tokenizer(
            pairs,
            padding=False,
            truncation="longest_first",
            return_attention_mask=False,
            max_length=MAX_LENGTH - len(self.prefix_tokens) - len(self.suffix_tokens),
        )
        for index, token_ids in enumerate(inputs["input_ids"]):
            inputs["input_ids"][index] = self.prefix_tokens + token_ids + self.suffix_tokens
        inputs = self.tokenizer.pad(inputs, padding=True, return_tensors="pt")
        inputs = {key: value.to(self.device) for key, value in inputs.items()}
        logits = self.model(**inputs).logits[:, -1, :]
        yes_no_logits = torch.stack(
            [logits[:, self.false_token_id], logits[:, self.true_token_id]],
            dim=1,
        )
        return torch.nn.functional.softmax(yes_no_logits, dim=1)[:, 1].float().cpu().tolist()

    def score(self, query: str, documents: list[str]) -> list[float]:
        with self.lock, torch.inference_mode():
            scores: list[float] = []
            for start in range(0, len(documents), BATCH_SIZE):
                scores.extend(self._score_batch(query, documents[start : start + BATCH_SIZE]))
        return scores


@asynccontextmanager
async def lifespan(app: FastAPI):
    app.state.reranker = QwenReranker()
    app.state.reranker.score("health check", ["health check"])
    yield


app = FastAPI(title="Diary Listener local Qwen3 reranker", lifespan=lifespan)


@app.get("/health")
def health() -> dict[str, str]:
    return {"status": "ok", "model": MODEL_ID}


@app.post("/rerank", response_model=RerankResponse)
def rerank(request: RerankRequest) -> RerankResponse:
    if request.model != MODEL_ID:
        raise HTTPException(status_code=400, detail="unsupported model")
    scores = app.state.reranker.score(request.query, request.documents)
    if len(scores) != len(request.documents) or any(not math.isfinite(score) for score in scores):
        raise HTTPException(status_code=502, detail="invalid model output")
    return RerankResponse(
        results=[
            RerankResult(index=index, relevance_score=score)
            for index, score in enumerate(scores)
        ]
    )
