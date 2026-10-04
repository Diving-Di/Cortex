import unittest
from compare_rag_runs import compare


class ComparisonTests(unittest.TestCase):
    def run_data(self, score=0.9, latency=100, digest="same", retrieval=True):
        return ({"total": 2, "failed": 0, "metrics": {key: score for key in ("hit_at_10", "mrr_after_rerank", "context_recall", "context_precision")}, "latency_p95": {"total_ms": latency}}, {"retrieval_only": retrieval}, digest)

    def test_quality_and_latency_regression_are_gated(self):
        report, failures = compare(self.run_data(), self.run_data(score=0.7, latency=150))
        self.assertIn("hit_at_10", failures)
        self.assertIn("total_p95_ms", failures)
        self.assertIn("成本未由当前产物完整采集", report)

    def test_incomparable_datasets_and_modes_are_rejected(self):
        for after in (self.run_data(digest="other"), self.run_data(retrieval=False)):
            with self.assertRaises(ValueError):
                compare(self.run_data(), after)

    def test_small_changes_pass_and_invalid_numbers_fail(self):
        self.assertEqual(compare(self.run_data(), self.run_data(score=0.89, latency=110))[1], [])
        with self.assertRaises(ValueError):
            compare(self.run_data(), self.run_data(score=float("nan")))


if __name__ == "__main__":
    unittest.main()
