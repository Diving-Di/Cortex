import logging
import re

_SECRET_PATTERNS = (
    re.compile(r"(?i)(authorization\s*[:=]\s*)(?:token|bearer)\s+[^\s,]+"),
    re.compile(r"(?i)((?:api[_-]?key|secret|password)\s*[:=]\s*)[^\s,]+"),
    re.compile(r"(?i)(postgresql(?:\+psycopg)?://[^:\s]+:)[^@\s]+(@)"),
)


def redact(value: str) -> str:
    result = value
    for pattern in _SECRET_PATTERNS:
        result = pattern.sub(lambda match: f"{match.group(1)}***{match.group(2) if match.lastindex and match.lastindex > 1 else ''}", result)
    return result


class RedactingFilter(logging.Filter):
    def filter(self, record: logging.LogRecord) -> bool:
        record.msg = redact(str(record.msg))
        if record.args:
            record.args = tuple(redact(str(arg)) for arg in record.args) if isinstance(record.args, tuple) else redact(str(record.args))
        return True


def configure_logging(level: str) -> None:
    logging.basicConfig(level=level, format="%(asctime)s %(levelname)s %(name)s %(message)s")
    for handler in logging.getLogger().handlers:
        handler.addFilter(RedactingFilter())
