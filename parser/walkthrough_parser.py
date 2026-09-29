#!/usr/bin/env python3
"""Extract a normalized Markdown article from HTML read on stdin."""

import json
import re
import sys

import trafilatura
from charset_normalizer import from_bytes
from lxml import html as lxml_html


charset_pattern = re.compile(
    rb"<meta\b[^>]*\bcharset\s*=\s*['\"]?\s*([a-zA-Z0-9._-]+)", re.IGNORECASE
)


def decode_html(content: bytes) -> str:
    """Decode HTML to Unicode before extracting Markdown."""
    declared = charset_pattern.search(content)
    if declared:
        try:
            return content.decode(declared.group(1).decode("ascii"), errors="replace")
        except LookupError:
            pass

    match = from_bytes(content).best()
    if match:
        return str(match)
    return content.decode("utf-8", errors="replace")


def extract_markdown(html: str) -> str:
    """Extract article Markdown, falling back to visible page text."""
    markdown = trafilatura.extract(
        html,
        output_format="markdown",
        include_comments=False,
        include_links=False,
        include_images=False,
        include_formatting=True,
        favor_precision=True,
    )
    if markdown:
        return markdown.strip()

    document = lxml_html.fromstring(html)
    for element in document.xpath("//script|//style|//noscript"):
        element.drop_tree()
    return "\n\n".join(text.strip() for text in document.xpath("//body//text()[normalize-space()]") if text.strip())


def main() -> int:
    content = sys.stdin.buffer.read()
    if not content.strip():
        print("empty HTML input", file=sys.stderr)
        return 2
    html = decode_html(content)

    markdown = extract_markdown(html)
    if not markdown:
        print("trafilatura could not extract article content", file=sys.stderr)
        return 3

    metadata = trafilatura.extract_metadata(html)
    title = metadata.title if metadata and metadata.title else ""
    language = metadata.language if metadata and metadata.language else ""
    result = {
        "title": title.strip(),
        "markdown": markdown.strip(),
        "metadata": {"language": language} if language else {},
    }
    json.dump(result, sys.stdout, ensure_ascii=False)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
