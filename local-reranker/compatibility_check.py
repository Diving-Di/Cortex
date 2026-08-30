import json
import os
import sys
import time

from app import DEVICE, MODEL_ID, RecipeReranker


def main() -> int:
    started = time.perf_counter()
    model = RecipeReranker()
    query = "苍穹计划的发布口令是什么？"
    documents = [
        "项目记录：苍穹计划的发布口令是青竹七号。",
        "今天上海天气晴朗，午后适合散步。",
        "The deployment password for Project Sky is Green Bamboo Number Seven.",
    ]
    values = model.score(query, documents)
    result = {
        "model": MODEL_ID,
        "device": DEVICE,
        "scores": values,
        "best_index": max(range(len(values)), key=values.__getitem__),
        "elapsed_seconds": round(time.perf_counter() - started, 3),
    }
    print(json.dumps(result, ensure_ascii=False))
    if result["best_index"] != 0 or values[0] <= values[1]:
        print("compatibility check failed: relevant Chinese passage was not ranked first", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
