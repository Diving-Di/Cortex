import math
import unittest

from pydantic import ValidationError

from app import MAX_DOCUMENTS, RerankRequest


class RequestValidationTest(unittest.TestCase):
    def test_rejects_too_many_documents(self) -> None:
        with self.assertRaises(ValidationError):
            RerankRequest(
                model="Qwen/Qwen3-Reranker-0.6B",
                query="query",
                documents=["document"] * (MAX_DOCUMENTS + 1),
            )

    def test_accepts_private_unicode_text_without_transforming_it(self) -> None:
        request = RerankRequest(
            model="Qwen/Qwen3-Reranker-0.6B",
            query="发布口令是什么？",
            documents=["发布口令是青竹七号。"],
        )
        self.assertEqual(request.documents[0], "发布口令是青竹七号。")
        self.assertTrue(math.isfinite(0.5))

    def test_rejects_empty_document(self) -> None:
        with self.assertRaises(ValidationError):
            RerankRequest(
                model="Qwen/Qwen3-Reranker-0.6B",
                query="query",
                documents=[""],
            )


if __name__ == "__main__":
    unittest.main()
