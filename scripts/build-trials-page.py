"""Build results/trials.html from the eval results.

Reads results/agent-*.jsonl (the tables) and results/kernel-replies.json
(every run with the kernel's replies rebuilt), and writes results/trials.html.

kernel-replies.json comes from `planproof-eval responses --json`, run with
the kernel version each run used: kernel 0.1.0 (commit 604361b) for the runs
before the digest check, 0.2.0 for the Haiku re-run. The kernel is
deterministic, so the rebuilt verdicts match the stored ones (checked).

    python3 scripts/build-trials-page.py
"""
import html
import json
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SP = os.path.dirname(os.path.abspath(__file__))  # this script and its template

CONFIGS = [  # file, label, kernel, prompt
    ("agent-claude-opus-5-5-high.jsonl", "Claude Opus 5.5", "0.1.0", "v1"),
    ("agent-claude-opus-5-5-high-prompt-v2.jsonl", "Claude Opus 5.5", "0.1.0", "v2"),
    ("agent-claude-sonnet-5-5-high.jsonl", "Claude Sonnet 5.5", "0.1.0", "v1"),
    ("agent-claude-sonnet-5-5-high-prompt-v2.jsonl", "Claude Sonnet 5.5", "0.1.0", "v2"),
    ("agent-claude-haiku-5-5-high-prompt-v2.jsonl", "Claude Haiku 5.5", "0.1.0", "v2"),
    ("agent-claude-haiku-5-5-high-kernel-0.2.0.jsonl", "Claude Haiku 5.5", "0.2.0", "v2"),
    ("agent-qwen2.5-1.5b-instruct.jsonl", "Qwen2.5-1.5B (local)", "0.1.0", "v1"),
]
PRICE = {"claude-opus-5-5": (4, 20, 0.20, 5), "claude-sonnet-5-5": (2, 10, 0.20, 2.5), "claude-haiku-5-5": (0.10, 0.50, 0.01, 0.125)}
INFEASIBLE = {"three-cordoned", "tight-two", "autoscaled-cart", "payments-up"}
TASK_ORDER = ["drain-two", "drain-n3", "upgrade-three", "scale-cart", "checkout-image", "restart-cart",
              "drain-then-image", "checkout-down", "tight-drain", "three-cordoned", "tight-two", "autoscaled-cart", "payments-up"]


def read(path):
    with open(path) as f:
        return [json.loads(l) for l in f if l.strip()]


def esc(s):
    return html.escape(str(s), quote=True)


def config_key(model_name):
    return model_name


# --- Results table -----------------------------------------------------------
rows = []
grid = {}  # task -> config index -> [pass bools]
for i, (fname, label, kver, prompt) in enumerate(CONFIGS):
    recs = read(os.path.join(ROOT, "results", fname))
    n = len(recs)
    solv = [r for r in recs if r["task"] not in INFEASIBLE]
    inf = [r for r in recs if r["task"] in INFEASIBLE]
    solved = sum(r["grade"]["outcome"] == "proved-correct" for r in solv)
    declined = sum(r["grade"]["outcome"] == "declared-no-safe-plan" for r in inf)
    first = sum(1 for r in recs if r["attempts"] and r["attempts"][0]["result"] == "Proved")
    repaired = sum(1 for r in recs if r["stopped"] == "proved" and r["attempts"][0]["result"] != "Proved")
    wrong = sum(r["grade"]["outcome"] == "proved-wrong-goal" for r in recs)
    falsedecl = sum(r["grade"]["outcome"] == "declared-but-solvable" for r in recs)
    u = [r["usage"] for r in recs]
    model = recs[0]["model"].split("/")[0]
    cost = None
    if model in PRICE:
        p = PRICE[model]
        cost = sum(x["Input"] * p[0] + x["Output"] * p[1] + x["CacheRead"] * p[2] + x["CacheWrite"] * p[3] for x in u) / 1e6
    runs_per_task = n // 13
    rows.append(dict(label=label, kernel=kver, prompt=prompt, n=n, runs=runs_per_task, solved=solved, nsolv=len(solv),
                     declined=declined, ninf=len(inf), first=first, repaired=repaired, wrong=wrong, falsedecl=falsedecl, cost=cost))
    for r in recs:
        grid.setdefault(r["task"], {}).setdefault(i, []).append(r["grade"]["pass"])

# --- Kernel responses --------------------------------------------------------
runs = json.load(open(os.path.join(ROOT, "results", "kernel-replies.json")))


def label_for(model_field):
    base = model_field.split("/")[0]
    names = {"claude-opus-5-5": "Claude Opus 5.5", "claude-sonnet-5-5": "Claude Sonnet 5.5",
             "claude-haiku-5-5": "Claude Haiku 5.5", "qwen2.5-1.5b-instruct": "Qwen2.5-1.5B (local)"}
    tag = ""
    if "prompt-v2" in model_field:
        tag = "prompt v2"
    elif "kernel-0.2.0" in model_field:
        tag = "prompt v2, kernel 0.2.0"
    elif base.startswith("claude"):
        tag = "prompt v1"
    return names.get(base, base), tag


def describe_step(s):
    op = s.get("op", "?")
    if op in ("cordon", "uncordon", "drain"):
        return f"{op} {s.get('node', '?')}"
    if op == "scale":
        return f"scale {s.get('deployment', '?')} to {s.get('replicas', '?')}"
    if op == "set-image":
        img = str(s.get("image", "?"))
        return f"set-image {s.get('deployment', '?')} {s.get('container', '')} → {short_image(img)}"
    if op == "rollout-restart":
        return f"restart {s.get('deployment', '?')}"
    if op == "wait":
        return f"wait for {', '.join(s.get('deployments') or [])} ({s.get('timeoutSeconds', '?')}s)"
    return json.dumps(s)


def short_image(img):
    if "@sha256:" in img:
        name, d = img.split("@sha256:", 1)
        return f"{name.split('/')[-1]}@sha256:{d[:6]}… ({len(d)} hex chars)"
    return img


def describe_goal(g):
    obj = g.get("node") or g.get("deployment") or "?"
    to = g.get("to")
    if g.get("field") == "image":
        to = short_image(str(to))
    c = f" {g['container']}" if g.get("container") else ""
    return f"{obj}{c} {g.get('field', '?')} → {to}"


OUTCOME = {
    "proved-correct": ("Solved", "ok"),
    "declared-no-safe-plan": ("Declined, correctly", "ok"),
    "proved-wrong-goal": ("Proved, wrong goal", "bad"),
    "declared-but-solvable": ("Declined a solvable task", "warn"),
    "not-proved": ("Not proved", "warn"),
    "error": ("Error", "bad"),
}

response_runs = [r for r in runs if any(e.get("response") for e in r["Exchanges"])]
filters = []
for r in response_runs:
    name, tag = label_for(r["Model"])
    key = f"{name} · {tag}" if tag else name
    if key not in filters:
        filters.append(key)


def render_run(r):
    name, tag = label_for(r["Model"])
    key = f"{name} · {tag}" if tag else name
    out_label, out_cls = OUTCOME.get(r["Outcome"], (r["Outcome"], "warn"))
    nresp = sum(1 for e in r["Exchanges"] if e.get("response"))
    parts = [f'<details class="run" data-filter="{esc(key)}">',
             f'<summary><span class="run-title"><span class="model">{esc(name)}</span>'
             f'<span class="tag">{esc(tag)}</span> <code>{esc(r["Task"])}</code></span>'
             f'<span class="run-meta"><span class="pill {out_cls}">{esc(out_label)}</span>'
             f'<span class="count">{nresp} kernel {"reply" if nresp == 1 else "replies"}</span></span></summary>',
             f'<p class="ticket"><span class="lbl">Ticket</span> {esc(r["Ticket"])}</p>']
    for e in r["Exchanges"]:
        plan = e.get("plan") or {}
        if not isinstance(plan, dict):
            plan = {}
        goal = plan.get("goal") or []
        steps = plan.get("steps") or []
        res_cls = {"Proved": "ok", "Refuted": "bad", "Unknown": "warn"}.get(e["result"], "warn")
        parts.append('<div class="exchange">')
        parts.append(f'<div class="ex-head"><span class="who">Model, submission {e["n"]}</span>'
                     f'<span class="verdict {res_cls}">{esc(e["result"])}</span></div>')
        parts.append('<div class="plan"><div><span class="lbl">Goal</span><ul>' +
                     "".join(f"<li>{esc(describe_goal(g))}</li>" for g in goal if isinstance(g, dict)) +
                     ('<li class="muted">(none)</li>' if not goal else "") + '</ul></div>')
        parts.append('<div><span class="lbl">Steps</span><ol>' +
                     "".join(f"<li>{esc(describe_step(s))}</li>" for s in steps if isinstance(s, dict)) +
                     ('<li class="muted">(none)</li>' if not steps else "") + '</ol></div></div>')
        if e.get("response"):
            resp = e["response"]
            expl = resp.get("explanation") or []
            parts.append('<div class="reply"><div class="ex-head"><span class="who">Kernel replies</span></div>')
            if expl:
                parts.append('<ul class="explain">' + "".join(f"<li>{esc(x)}</li>" for x in expl) + "</ul>")
            parts.append('<details class="raw"><summary>Exact message (JSON)</summary><pre>' +
                         esc(json.dumps(resp, indent=2, ensure_ascii=False)) + "</pre></details></div>")
        else:
            parts.append('<p class="nothing">Nothing sent back: ' +
                         ("the plan is proved, so the loop stops." if e["result"] == "Proved" else "that was the last submission allowed.") + "</p>")
        parts.append("</div>")
    if r.get("Reminder"):
        parts.append(f'<p class="reminder"><span class="lbl">Loop reminder, once</span> {esc(r["Reminder"])}</p>')
    if r.get("Final"):
        final = r["Final"].strip()
        parts.append(f'<div class="final"><span class="lbl">Model\'s last words (unchecked)</span><p>{esc(final[:700])}{"…" if len(final) > 700 else ""}</p></div>')
    parts.append("</details>")
    return "\n".join(parts)


def pct(a, b):
    return f"{a}/{b}"


table_rows = []
for r in rows:
    cost = "free (local GPU)" if r["cost"] is None else f"${r['cost']:.2f}"
    table_rows.append(
        f'<tr><th scope="row">{esc(r["label"])}<span class="sub">prompt {r["prompt"]}, kernel {r["kernel"]}</span></th>'
        f'<td>{r["runs"]}×13</td><td>{pct(r["solved"], r["nsolv"])}</td><td>{pct(r["declined"], r["ninf"])}</td>'
        f'<td>{r["first"]}</td><td>{r["repaired"]}</td>'
        f'<td class="{"bad" if r["wrong"] else ""}">{r["wrong"]}</td><td class="{"warn" if r["falsedecl"] else ""}">{r["falsedecl"]}</td>'
        f'<td>{cost}</td></tr>')

grid_head = "".join(f'<th scope="col"><span>{esc(c[1].replace("Claude ", "").replace(" (local)", ""))}</span>'
                    f'<span class="sub">{c[3]}{", k0.2" if c[2] == "0.2.0" else ""}</span></th>' for c in CONFIGS)
grid_rows = []
for t in TASK_ORDER:
    cells = []
    for i in range(len(CONFIGS)):
        v = grid.get(t, {}).get(i, [])
        k = sum(v)
        cls = "ok" if k == len(v) else ("bad" if k == 0 else "warn")
        cells.append(f'<td class="cell {cls}">{k}/{len(v)}</td>')
    kind = '<span class="sub">no safe plan</span>' if t in INFEASIBLE else ""
    grid_rows.append(f'<tr><th scope="row"><code>{esc(t)}</code>{kind}</th>{"".join(cells)}</tr>')

chips = "".join(f'<button type="button" class="chip" aria-pressed="false" data-chip="{esc(f)}">{esc(f)}</button>' for f in filters)
total_runs = sum(r["n"] for r in rows)
total_replies = sum(1 for r in runs for e in r["Exchanges"] if e.get("response"))

page = open(os.path.join(SP, "trials.template.html")).read()
page = (page.replace("{{TABLE}}", "\n".join(table_rows)).replace("{{GRID_HEAD}}", grid_head)
        .replace("{{GRID}}", "\n".join(grid_rows)).replace("{{CHIPS}}", chips)
        .replace("{{RUNS}}", "\n".join(render_run(r) for r in response_runs))
        .replace("{{TOTAL_RUNS}}", str(total_runs)).replace("{{TOTAL_REPLIES}}", str(total_replies))
        .replace("{{REPLY_RUNS}}", str(len(response_runs))))
open(os.path.join(ROOT, "results", "trials.html"), "w").write(page)
print("runs", total_runs, "replies", total_replies, "runs with replies", len(response_runs), "bytes", len(page))
