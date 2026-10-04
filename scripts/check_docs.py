"""Check repository-relative links in the maintained docs and root README."""
import re
import sys
from pathlib import Path
from urllib.parse import unquote

root = Path(__file__).resolve().parent.parent
files = [root / "README.md", *sorted((root / "docs").rglob("*.md"))]
errors = []
for source in files:
    text = source.read_text(encoding="utf-8")
    # Examples inside fenced code are not documentation navigation links.
    text = re.sub(r"```.*?```", "", text, flags=re.S)
    for match in re.finditer(r"\[[^\]]*\]\(([^)]+)\)", text):
        target = match.group(1).strip().split(' "', 1)[0].strip("<>")
        if not target or target.startswith(("#", "/")) or re.match(r"^[a-zA-Z][\w+.-]*:", target):
            continue
        path = unquote(target.split("#", 1)[0].split("?", 1)[0])
        if not (source.parent / path).exists():
            errors.append(f"{source.relative_to(root)}: missing {target}")
if errors:
    print("\n".join(errors))
    sys.exit(1)
print(f"Documentation links passed: {len(files)} Markdown files.")
