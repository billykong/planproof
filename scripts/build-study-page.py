"""Build results/study/index.html, the reviewer study page (PRD E-22).

Embeds results/study/items.json (from `planproof-eval study-items`) in
scripts/study.template.html. The page is published as an artifact with a
shared database; each reviewer's decisions are one document,
responses/<their anonymous id>, readable only by the owner.

    python3 scripts/build-study-page.py
"""
import json
import os

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
items = json.load(open(os.path.join(ROOT, "results", "study", "items.json")))
# Only what the page shows; the source names and verdicts stay out.
SHOWN = ("id", "ticket", "raw", "packet", "answer", "why")
items["items"] = [{k: it[k] for k in SHOWN} for it in items["items"]]
data = json.dumps(items, ensure_ascii=False).replace("</", "<\\/")
page = open(os.path.join(ROOT, "scripts", "study.template.html")).read().replace("{{ITEMS}}", data)
open(os.path.join(ROOT, "results", "study", "index.html"), "w").write(page)
print("items", len(items["items"]), "bytes", len(page))
