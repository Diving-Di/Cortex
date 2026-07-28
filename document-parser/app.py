import json
import os
import re
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pymupdf
import pymupdf4llm
from docx import Document

MAX_FILE_BYTES = int(os.getenv("PARSER_MAX_FILE_BYTES", str(50 * 1024 * 1024)))
MAX_CHARS = int(os.getenv("PARSER_MAX_CHARS", "5000000"))
MAX_PAGES = int(os.getenv("PARSER_MAX_PAGES", "500"))
ALLOWED = {"txt", "docx", "pdf"}


class ParseError(Exception):
    def __init__(self, code: str, message: str, status: int = 422):
        super().__init__(message)
        self.code, self.message, self.status = code, message, status


def clean(value: str) -> str:
    value = value.replace("\r\n", "\n").replace("\r", "\n").replace("\x00", "")
    return "\n".join(line.rstrip() for line in value.splitlines()).strip()


def parse_txt(path: str):
    try:
        text = Path(path).read_text(encoding="utf-8-sig")
    except UnicodeDecodeError as exc:
        raise ParseError("DOCUMENT_PARSE_FAILED", "TXT 必须使用 UTF-8 编码") from exc
    text = clean(text)
    return [{"page": 1, "markdown": text}]


def table_markdown(table) -> str:
    rows = [[clean(cell.text).replace("|", "\\|").replace("\n", "<br>") for cell in row.cells]
            for row in table.rows]
    if not rows:
        return ""
    width = max(len(row) for row in rows)
    rows = [row + [""] * (width - len(row)) for row in rows]
    return "\n".join([
        "| " + " | ".join(rows[0]) + " |",
        "| " + " | ".join(["---"] * width) + " |",
        *["| " + " | ".join(row) + " |" for row in rows[1:]],
    ])


def parse_docx(path: str):
    try:
        document = Document(path)
    except Exception as exc:
        raise ParseError("DOCUMENT_PARSE_FAILED", "DOCX 解析失败") from exc
    parts = []
    for block in document.element.body:
        tag = block.tag.rsplit("}", 1)[-1]
        if tag == "p":
            paragraph = next((p for p in document.paragraphs if p._p is block), None)
            if paragraph is None or not clean(paragraph.text):
                continue
            text = clean(paragraph.text)
            style = (paragraph.style.name if paragraph.style else "").lower()
            match = re.search(r"heading\s*(\d+)", style)
            if match:
                text = f"{'#' * min(6, max(1, int(match.group(1))))} {text}"
            elif paragraph._p.pPr is not None and paragraph._p.pPr.numPr is not None:
                text = f"- {text}"
            parts.append(text)
        elif tag == "tbl":
            table = next((t for t in document.tables if t._tbl is block), None)
            if table is not None:
                rendered = table_markdown(table)
                if rendered:
                    parts.append(rendered)
    return [{"page": 1, "markdown": "\n\n".join(parts)}]


def parse_pdf(path: str):
    try:
        doc = pymupdf.open(path)
        if doc.needs_pass:
            raise ParseError("DOCUMENT_ENCRYPTED", "PDF 已加密")
        count = doc.page_count
        doc.close()
        if count > MAX_PAGES:
            raise ParseError("DOCUMENT_PARSE_LIMIT", "PDF 页数超过限制", 413)
        chunks = pymupdf4llm.to_markdown(path, page_chunks=True, show_progress=False)
    except ParseError:
        raise
    except Exception as exc:
        raise ParseError("DOCUMENT_PARSE_FAILED", "PDF 解析失败") from exc
    pages = []
    for index, chunk in enumerate(chunks):
        text = clean(chunk.get("text", "") if isinstance(chunk, dict) else str(chunk))
        pages.append({"page": index + 1, "markdown": text})
    meaningful = sum(ch.isalnum() for page in pages for ch in page["markdown"])
    if meaningful < 8:
        raise ParseError("DOCUMENT_OCR_REQUIRED", "PDF 不包含足够的可提取文本")
    return pages


def parse(path: str, kind: str):
    pages = {"txt": parse_txt, "docx": parse_docx, "pdf": parse_pdf}[kind](path)
    characters = sum(len(page["markdown"]) for page in pages)
    if characters > MAX_CHARS:
        raise ParseError("DOCUMENT_PARSE_LIMIT", "提取文本超过限制", 413)
    if not any(page["markdown"].strip() for page in pages):
        raise ParseError("DOCUMENT_OCR_REQUIRED", "文档不包含可提取文本")
    return {"pages": pages, "page_count": len(pages), "character_count": characters,
            "warnings": []}


class Handler(BaseHTTPRequestHandler):
    server_version = "CortexDocumentParser/1"

    def log_message(self, fmt, *args):
        print(json.dumps({"event": "http", "message": fmt % args}, ensure_ascii=False))

    def send_json(self, status, payload):
        body = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self.send_json(200, {"status": "ok"})
        else:
            self.send_json(404, {"code": "NOT_FOUND", "message": "not found"})

    def do_POST(self):
        if self.path != "/v1/parse":
            self.send_json(404, {"code": "NOT_FOUND", "message": "not found"})
            return
        kind = self.headers.get("X-Document-Format", "").lower().lstrip(".")
        try:
            if kind not in ALLOWED:
                raise ParseError("DOCUMENT_UNSUPPORTED_TYPE", "不支持的文档格式")
            length = int(self.headers.get("Content-Length", "0"))
            if length <= 0 or length > MAX_FILE_BYTES:
                raise ParseError("DOCUMENT_PARSE_LIMIT", "文件大小超过限制", 413)
            with tempfile.NamedTemporaryFile(suffix=f".{kind}") as target:
                remaining = length
                while remaining:
                    chunk = self.rfile.read(min(1024 * 1024, remaining))
                    if not chunk:
                        raise ParseError("DOCUMENT_PARSE_FAILED", "文件传输不完整")
                    target.write(chunk)
                    remaining -= len(chunk)
                target.flush()
                self.send_json(200, parse(target.name, kind))
        except ParseError as exc:
            self.send_json(exc.status, {"code": exc.code, "message": exc.message})
        except Exception:
            self.send_json(500, {"code": "DOCUMENT_PARSE_FAILED", "message": "文档解析失败"})


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", int(os.getenv("PORT", "8090"))), Handler).serve_forever()
