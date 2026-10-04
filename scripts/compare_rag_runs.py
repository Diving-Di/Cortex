"""Compare existing Go rag-eval artifacts without emitting private case bodies."""
import argparse
import hashlib
import json
import math
from pathlib import Path


def load_run(directory):
    directory = Path(directory)
    summary = json.loads((directory / "summary.json").read_text(encoding="utf-8"))
    config = json.loads((directory / "config.json").read_text(encoding="utf-8"))
    cases = [json.loads(line) for line in (directory / "cases.jsonl").read_text(encoding="utf-8").splitlines() if line.strip()]
    if not cases or len({case["id"] for case in cases}) != len(cases):
        raise ValueError("Case set is empty or contains duplicate IDs")
    fingerprint = [{key: case.get(key) for key in ("id", "query", "reference_answer", "source_paths", "tags")} for case in cases]
    fingerprint.sort(key=lambda case: case["id"])
    digest = hashlib.sha256(json.dumps(fingerprint, sort_keys=True, ensure_ascii=False).encode()).hexdigest()
    if summary.get("total") != len(cases):
        raise ValueError("Summary count does not match case artifacts")
    return summary, config, digest


def compare(before, after, quality_tolerance=0.02, latency_ratio=1.25):
    old, old_config, old_hash = before
    new, new_config, new_hash = after
    if old_hash != new_hash:
        raise ValueError("Frozen case content differs; runs are not comparable")
    if old_config.get("retrieval_only") != new_config.get("retrieval_only"):
        raise ValueError("Retrieval-only and full-generation runs are not comparable")
    failures, rows = [], []
    for key in ("hit_at_10", "mrr_after_rerank", "context_recall", "context_precision"):
        a, b = float(old["metrics"][key]), float(new["metrics"][key])
        if not all(math.isfinite(value) and 0 <= value <= 1 for value in (a, b)):
            raise ValueError("Invalid quality metric")
        rows.append((key, a, b))
        if a - b > quality_tolerance + 1e-9:
            failures.append(key)
    a, b = float(old["latency_p95"]["total_ms"]), float(new["latency_p95"]["total_ms"])
    if not all(math.isfinite(value) and value >= 0 for value in (a, b)):
        raise ValueError("Invalid latency metric")
    rows.append(("total_p95_ms", a, b))
    if a > 0 and b > a * latency_ratio:
        failures.append("total_p95_ms")
    if new["failed"] > old["failed"]:
        failures.append("failed_cases")
    changes = [key for key in sorted(set(old_config) | set(new_config)) if old_config.get(key) != new_config.get(key)]
    report = "# RAG 回归对照\n\n"
    report += f"冻结集 SHA-256：{old_hash}\n\n样本：{new['total']}；结果：{'未通过' if failures else '通过'}\n\n"
    report += "| 指标 | 基线 | 当前 |\n| --- | ---: | ---: |\n"
    report += "".join(f"| {name} | {a:.4f} | {b:.4f} |\n" for name, a, b in rows)
    report += f"\n质量允许下降 {quality_tolerance:.4f}；P95 最大倍数 {latency_ratio:.2f}。\n"
    report += "配置变化字段：" + (", ".join(changes) or "无") + "。\n"
    report += "未通过项：" + (", ".join(failures) or "无") + "。\n"
    report += "\nToken、调用次数和供应商成本未由当前产物完整采集，不能据此宣称成本降低。离线结果不替代线上 SSE 验收。\n"
    return report, failures


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("baseline")
    parser.add_argument("candidate")
    parser.add_argument("--output", required=True)
    parser.add_argument("--quality-tolerance", type=float, default=0.02)
    parser.add_argument("--max-latency-ratio", type=float, default=1.25)
    args = parser.parse_args()
    if not 0 <= args.quality_tolerance <= 1 or not math.isfinite(args.max_latency_ratio) or args.max_latency_ratio < 1:
        parser.error("Invalid regression thresholds")
    try:
        report, failures = compare(load_run(args.baseline), load_run(args.candidate), args.quality_tolerance, args.max_latency_ratio)
        output = Path(args.output)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(report, encoding="utf-8")
    except (OSError, ValueError, KeyError, TypeError):
        parser.exit(2, "Cannot compare artifacts: check file structure, frozen cases and metrics.\n")
    print(f"Comparison {'failed' if failures else 'passed'}; {len(failures)} regression gates failed.")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
