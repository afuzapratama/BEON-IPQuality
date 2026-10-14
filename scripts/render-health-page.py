#!/usr/bin/env python3
"""Embed the standalone health page in the Caddy template without placeholder expansion."""
import base64
from pathlib import Path

root = Path(__file__).resolve().parent.parent
page = base64.b64encode((root / "web/health.html").read_bytes()).decode()
config = root / "configs/caddy/beon-production.caddyfile"
text = config.read_text()
start = text.index("  respond `", text.index(" @healthPage"))
end = text.index("` 200", start) + len("` 200")
# Caddy expands braces in respond bodies; encoding keeps JavaScript/CSS intact.
shell = ('<!doctype html><html lang="id"><meta charset="utf-8"><title>BEON Status</title>'
         '<script>document.write(new TextDecoder().decode(Uint8Array.from(atob("'
         + page + '"),c=>c.charCodeAt(0))))</script>'
         '<noscript>Aktifkan JavaScript atau buka /health?format=json.</noscript></html>')
config.write_text(text[:start] + "  respond `" + shell + "` 200" + text[end:])
