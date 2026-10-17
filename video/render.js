// Renders the frames, in the style of the blog post's card, and the ffmpeg
// concat list that times them to the narration.
//   node render.js OUT_DIR             the video (scenes.js), 1920x1080
//   node render.js OUT_DIR SHORT_ID    a Short (shorts.js), 1080x1920
//   node render.js --dump [SHORT_ID]   print the scenes as JSON (for tts.py)
// Needs OUT_DIR/timings.json from tts.py. Frames are SVG rendered with resvg;
// text is DejaVu (resvg-js ignores weights in variable fonts).
const fs = require('fs');
const path = require('path');

const C = { bg: '#0f172a', grid: '#1e293b', fg: '#f8fafc', muted: '#cbd5e1', dim: '#94a3b8', teal: '#2dd4bf', amber: '#fbbf24', green: '#4ade80', greenbg: '#14532d', box: '#1e293b', edge: '#334155', term: '#0b1220', warn: '#3b2f12' };
const sans = 'DejaVu Sans', mono = 'DejaVu Sans Mono';
let W = 1920, H = 1080, X = 128, SHORT = false;
const layout = short => { SHORT = short; [W, H, X] = short ? [1080, 1920, 72] : [1920, 1080, 128]; };

const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const t = (x, y, s, size, fill, font = sans, weight = 400, extra = '') =>
  `<text x="${x}" y="${y}" font-family="${font}" font-size="${size}" font-weight="${weight}" fill="${fill}" ${extra}>${esc(s)}</text>`;
const frame = inner => `<svg xmlns="http://www.w3.org/2000/svg" width="${W}" height="${H}" viewBox="0 0 ${W} ${H}">
<defs><pattern id="g" width="64" height="64" patternUnits="userSpaceOnUse"><path d="M64 0H0V64" fill="none" stroke="${C.grid}" stroke-width="1.5"/></pattern>
</defs>
<rect width="${W}" height="${H}" fill="${C.bg}"/><rect width="${W}" height="${H}" fill="url(#g)" opacity="0.6"/>
<rect x="0" y="0" width="16" height="${H}" fill="${C.teal}"/>${inner}</svg>`;
// Shorts keep text out of the bottom ~420 px and the right edge, where
// YouTube draws its buttons and the caption.
const TOP = () => SHORT ? 260 : 150;
const label = s => { const size = SHORT ? 30 : 36; return wrap(s, W - 2 * X, size, 0.602 + 5 / size).map((w, i) => t(X, TOP() + i * size * 1.4, w, size, C.teal, mono, 700, 'letter-spacing="5"')).join(''); };
const footer = s => t(X, SHORT ? 1480 : 1010, s, 28, C.dim, mono);

// Greedy wrap by estimated width: DejaVu Sans averages ~0.55em a character,
// bold ~0.62em, mono exactly 0.602em.
function wrap(s, px, size, factor = 0.55) {
  const max = Math.floor(px / (size * factor)), out = [];
  let line = '';
  for (const w of String(s).split(' ')) {
    if (line && (line + ' ' + w).length > max) { out.push(line); line = w; } else line = line ? line + ' ' + w : w;
  }
  if (line) out.push(line);
  return out;
}
const lines = (x, y, s, px, size, fill, weight = 400, gap = 1.35, factor) => {
  const ws = wrap(s, px, size, factor ?? (weight === 700 ? 0.62 : 0.55));
  return { svg: ws.map((w, i) => t(x, y + i * size * gap, w, size, fill, sans, weight)).join(''), h: ws.length * size * gap };
};

// The card's motif: AI plan -> checker -> PROVED, top to bottom.
function stack(cx, y, k = 1, verdict = 'PROVED') {
  const box = (bx, by, w, h, fill, stroke, a, b, col) => `<rect x="${bx - w / 2}" y="${by}" width="${w}" height="${h}" rx="${14 * k}" fill="${fill}" stroke="${stroke}" stroke-width="${4 * k}"/>`
    + t(bx, by + (b ? h * 0.45 : h * 0.6), a, 30 * k, col, mono, 700, 'text-anchor="middle"')
    + (b ? t(bx, by + h * 0.78, b, 21 * k, C.dim, mono, 400, 'text-anchor="middle"') : '');
  const arrow = (y1, y2) => `<path d="M${cx} ${y1}V${y2 - 4 * k}" stroke="${C.teal}" stroke-width="${4 * k}"/><path d="M${cx - 13 * k} ${y2 - 18 * k}L${cx} ${y2}L${cx + 13 * k} ${y2 - 18 * k}" fill="none" stroke="${C.teal}" stroke-width="${4 * k}"/>`;
  const refuted = verdict !== 'PROVED';
  return box(cx, y, 170 * k, 96 * k, C.amber, C.amber, 'AI plan', '', C.bg)
    + arrow(y + 96 * k, y + 190 * k)
    + box(cx, y + 190 * k, 270 * k, 112 * k, C.bg, C.teal, 'checker', 'no AI · 1,700 lines', C.teal)
    + arrow(y + 302 * k, y + 396 * k)
    + box(cx, y + 396 * k, 200 * k, 80 * k, refuted ? C.warn : C.greenbg, refuted ? C.amber : C.green, verdict, '', refuted ? C.amber : C.green);
}

const V = {
  title() {
    return frame(label('PLANPROOF · OPEN SOURCE · MIT')
      + t(X, 330, 'Don’t trust the AI’s plan.', 84, C.fg, sans, 700)
      + t(X, 445, 'Trust a checker small', 84, C.fg, sans, 700)
      + t(X, 560, 'enough to read.', 84, C.fg, sans, 700)
      + t(X, 690, 'An AI writes the change plan; a small, rule-based', 42, C.muted)
      + t(X, 750, 'checker proves it or says why not, in plain English.', 42, C.muted)
      + footer('github.com/billykong/planproof')
      + stack(1620, 270, 1.15));
  },

  hook(v) {
    let s = label('PLANPROOF'), y = 480;
    for (const l of v.lines) { const r = lines(X, y, l, W - 2 * X, 84, C.fg, 700, 1.2, 0.62); s += r.svg; y += r.h; }
    s += lines(X, y + 50, v.kicker, W - 2 * X, 84, C.amber, 700, 1.2, 0.62).svg;
    return frame(s + stack(W / 2, 960, 1.1, v.verdict || 'REFUTED'));
  },

  statement(v, b) {
    const size = SHORT ? 66 : 56, px = W - 2 * X - (SHORT ? 40 : 0);
    let s = label(v.label), y = SHORT ? 600 : 300;
    v.lines.forEach((l, i) => {
      const on = i < b.show, last = i === b.show - 1;
      const r = lines(X, y, l, px, size, on ? (last ? C.fg : C.muted) : C.grid, 700, 1.38, 0.6);
      s += r.svg; y += r.h + (SHORT ? 60 : 34);
    });
    if (v.question && b.show > v.lines.length) {
      y += 20;
      const qs = wrap(v.question, px - 40, 46, 0.62);
      s += `<rect x="${X}" y="${y - 50}" width="10" height="${qs.length * 64}" fill="${C.amber}"/>`;
      qs.forEach(w => { s += t(X + 40, y, w, 46, C.amber, sans, 700); y += 64; });
    }
    return frame(s);
  },

  cards(v, b) {
    let s = label(v.label) + t(X, 250, v.title, 64, C.fg, sans, 700);
    const two = v.cards.length === 2, ch = two ? 420 : 290;
    v.cards.forEach(([n, head, body], i) => {
      const x = X + (i % 2) * 840, y = 320 + Math.floor(i / 2) * 330, on = i < b.show, col = n === 'AI' ? C.amber : C.teal;
      s += `<rect x="${x}" y="${y}" width="800" height="${ch}" rx="20" fill="${C.box}" stroke="${on ? col : C.edge}" stroke-width="${on ? 4 : 2}" opacity="${on ? 1 : 0.4}"/>`;
      if (!on) return;
      s += t(x + 44, y + 92, n, 60, col, mono, 700) + t(x + (n.length > 1 ? 150 : 120), y + 84, head, 36, C.fg, sans, 700);
      wrap(body, 700, two ? 40 : 32).forEach((w, j) => { s += t(x + 44, y + (two ? 190 : 160) + j * (two ? 58 : 46), w, two ? 40 : 32, C.muted); });
    });
    return frame(s);
  },

  flow(v, b) {
    const BH = 120;
    const N = {
      ticket: [240, 470, 220, 'Ticket', ''],
      ai: [580, 470, 280, 'AI agent', 'writes a plan'],
      snapshot: [1130, 260, 300, 'Snapshot', 'of the cluster'],
      policy: [1540, 260, 240, 'Policy', ''],
      checker: [1130, 470, 340, 'Checker', 'no AI · ~1,700 lines'],
      person: [1130, 700, 340, 'Person approves', 'reads the report'],
      executor: [1130, 900, 340, 'Executor', 'runs proved plans'],
      cluster: [1610, 900, 260, 'Cluster', ''],
    };
    const hl = new Set(b.hl || []);
    const side = (k, d) => { const [x, y, w] = N[k]; return d === 'r' ? [x + w / 2, y] : d === 'l' ? [x - w / 2, y] : d === 'u' ? [x, y - BH / 2] : [x, y + BH / 2]; };
    // resvg ignores a marker's orient="auto", so each arrowhead is drawn
    // here, pointing along the last segment of its line.
    const arrow = (pts, col = C.dim) => {
      const [[px, py], [x, y]] = pts.slice(-2), a = Math.atan2(y - py, x - px), L = 18, Wd = 9;
      const bx = x - L * Math.cos(a), by = y - L * Math.sin(a), nx = -Math.sin(a) * Wd, ny = Math.cos(a) * Wd;
      return `<polyline points="${pts.slice(0, -1).concat([[bx, by]]).map(p => p.join(',')).join(' ')}" fill="none" stroke="${col}" stroke-width="3"/>`
        + `<path d="M${x} ${y}L${bx + nx} ${by + ny}L${bx - nx} ${by - ny}Z" fill="${col}"/>`;
    };
    const edge = (a, ad, z, zd) => {
      const [x1, y1] = side(a, ad), [x2, y2] = side(z, zd);
      return arrow(ad === 'u' || ad === 'd' || y1 === y2 ? [[x1, y1], [x2, y2]] : [[x1, y1], [x2, y1], [x2, y2]]);
    };
    const cl = N.checker[0] - 170, cr = N.checker[0] + 170, ar = N.ai[0] + 140;
    let s = label('HOW IT FITS TOGETHER')
      + edge('ticket', 'r', 'ai', 'l')
      + arrow([[ar, N.ai[1] - 22], [cl, N.ai[1] - 22]])
      + arrow([[cl, N.ai[1] + 22], [ar, N.ai[1] + 22]], C.amber)
      + t(845, 432, 'plan', 24, C.dim, mono, 400, 'text-anchor="middle"')
      + t(845, 580, 'refuted: step + reason', 22, C.amber, mono, 400, 'text-anchor="middle"')
      + edge('snapshot', 'd', 'checker', 'u')
      + arrow([[N.policy[0], N.policy[1] + BH / 2], [N.policy[0], N.checker[1]], [cr, N.checker[1]]])
      + edge('checker', 'd', 'person', 'u') + t(1150, 600, 'proved + report', 24, C.green, mono)
      + edge('person', 'd', 'executor', 'u') + edge('executor', 'r', 'cluster', 'l');
    for (const [k, [x, y, w, a, sub]] of Object.entries(N)) {
      const on = hl.has(k), base = k === 'ai' ? C.amber : C.teal, op = hl.size && !on ? 0.35 : 1;
      s += `<g opacity="${op}"><rect x="${x - w / 2}" y="${y - BH / 2}" width="${w}" height="${BH}" rx="16" fill="${C.box}" stroke="${on ? C.amber : base}" stroke-width="${on ? 6 : 3}"/>`
        + t(x, sub ? y - 4 : y + 10, a, 30, C.fg, mono, 700, 'text-anchor="middle"')
        + (sub ? t(x, y + 36, sub, 23, C.dim, sans, 400, 'text-anchor="middle"') : '') + '</g>';
    }
    return frame(s);
  },

  terminal(v, b, cap, upto) {
    // Long lines wrap (indented) instead of being cut off.
    const size = SHORT ? 27 : 28, lh = SHORT ? 38 : 37, cw = size * 0.602;
    const top = SHORT ? 360 : 190, h = SHORT ? 1080 : 850, max = Math.floor((W - 2 * X - 64) / cw);
    let rows = [];
    cap.lines.slice(0, upto).forEach(l => {
      const ind = (l.match(/^\s*/)[0].length) + 2;
      let rest = l, first = true;
      while (rest.length) {
        const room = first ? max : max - ind;
        let cut = rest.length <= room ? rest.length : rest.lastIndexOf(' ', room);
        if (cut <= 0) cut = room;
        rows.push([(first ? '' : ' '.repeat(ind)) + rest.slice(0, cut), l]);
        rest = rest.slice(cut).replace(/^ /, ''); first = false;
      }
      if (!l.length) rows.push(['', l]);
    });
    rows = rows.slice(-Math.floor((h - 100) / lh));
    let s = label(v.label);
    s += `<rect x="${X}" y="${top}" width="${W - 2 * X}" height="${h}" rx="18" fill="${C.term}" stroke="${C.edge}" stroke-width="2"/>`
      + `<rect x="${X}" y="${top}" width="${W - 2 * X}" height="56" rx="18" fill="${C.box}"/><rect x="${X}" y="${top + 36}" width="${W - 2 * X}" height="20" fill="${C.box}"/>`
      + [0, 1, 2].map(i => `<circle cx="${X + 36 + i * 30}" cy="${top + 28}" r="9" fill="${[C.amber, C.dim, C.teal][i]}"/>`).join('')
      + t(SHORT ? X + 140 : W / 2, top + 37, cap.title, SHORT ? 20 : 24, C.dim, mono, 400, SHORT ? '' : 'text-anchor="middle"');
    rows.forEach(([r, l], i) => {
      const col = /^\$ /.test(l) ? C.fg : /PROVED/.test(l) ? C.green : /^(CAUTION|During step|The plan makes|The number of)/.test(l) ? C.amber : /does not check that this goal/.test(l) ? C.teal : C.muted;
      const bold = /^(\$ |CAUTION|\d\. [A-Z]{3})/.test(l) || /PROVED/.test(l) ? 700 : 400;
      s += t(X + 32, top + 100 + i * lh, r, size, col, mono, bold, 'xml:space="preserve"');
    });
    return frame(s);
  },

  table(v, b) {
    let s = label(v.label) + t(X, 250, v.title, 60, C.fg, sans, 700);
    const tw = W - 2 * X, xs = v.widths.reduce((a, w, i) => [...a, a[i] + w * tw], [X]);
    v.cols.forEach((c, i) => { s += t(xs[i] + 24, 340, c.toUpperCase(), 24, C.dim, mono, 700, 'letter-spacing="3"'); });
    v.rows.forEach((r, i) => {
      if (i >= b.show) return;
      const y = 420 + i * 86;
      s += `<rect x="${X}" y="${y - 54}" width="${tw}" height="76" rx="10" fill="${i % 2 ? C.bg : C.box}"/>`;
      r.forEach((c, j) => { s += t(xs[j] + 24, y, c, 38, j === 0 ? C.fg : j === r.length - 1 && v.cols[1] === 'Predicted' ? C.teal : C.muted, j === 0 ? sans : mono, j === 0 ? 400 : 700); });
    });
    if (v.note) s += footer(v.note);
    return frame(s);
  },

  numbers(v, b) {
    let s = label(v.label);
    if (v.title) s += SHORT ? lines(X, 420, v.title, W - 2 * X, 52, C.fg, 700).svg : t(X, 250, v.title, 60, C.fg, sans, 700);
    const n = v.stats.length, colw = (W - 2 * X) / n;
    v.stats.forEach(([big, cap, col], i) => {
      const on = i < b.show, c = C[col];
      const x = SHORT ? X : X + i * colw, y = SHORT ? 560 + i * 400 : 420;
      const size = SHORT ? 200 : (big.length > 3 ? 170 : 220);
      s += `<g opacity="${on ? 1 : 0.15}">` + t(x, y + size * 0.8, big, size, c, mono, 700)
        + lines(x, y + size + 70, cap, SHORT ? W - 2 * X : colw - 60, SHORT ? 44 : 38, on ? C.muted : C.dim).svg + '</g>';
    });
    if (v.note && b.show > n) s += t(X, SHORT ? 1420 : 930, v.note, SHORT ? 44 : 48, C.amber, sans, 700);
    return frame(s);
  },

  list(v, b) {
    let s = label(v.label) + t(X, 250, v.title, 60, C.fg, sans, 700);
    const step = v.items.length > 5 ? 80 : 120, size = v.items.length > 5 ? 38 : 46;
    v.items.forEach((it, i) => {
      if (i >= b.show) return;
      const y = 370 + i * step, cur = i === b.show - 1 || v.items.length > 5 && i >= b.show - 4;
      const m = it.match(/^(\d{4})\s+(.*)$/);
      if (m) {
        s += t(X, y, m[1], size, cur ? C.amber : C.teal, mono, 700) + t(X + 170, y, m[2], size, cur ? C.fg : C.muted);
      } else {
        s += `<rect x="${X}" y="${y - 46}" width="64" height="62" rx="10" fill="${cur ? C.amber : C.box}"/>`
          + t(X + 32, y - 2, i + 1, 36, cur ? C.bg : C.teal, mono, 700, 'text-anchor="middle"')
          + t(X + 100, y, it, size, cur ? C.fg : C.muted, sans, 700);
      }
    });
    return frame(s);
  },

  end() {
    const cw = SHORT ? W - 2 * X : 1100;
    const card = (y, k, v) => `<rect x="${X}" y="${y}" width="${cw}" height="130" rx="18" fill="${C.box}" stroke="${C.teal}" stroke-width="3"/>`
      + t(X + 40, y + 52, k, 26, C.teal, mono, 700, 'letter-spacing="3"') + t(X + 40, y + 100, v, SHORT ? 34 : 38, C.fg, mono, 700);
    if (SHORT) {
      return frame(label('PLANPROOF')
        + lines(X, 420, 'Checking AI-written change plans', W - 2 * X, 64, C.fg, 700).svg
        + card(640, 'CODE · MIT', 'github.com/billykong/planproof')
        + card(810, 'WRITE-UP', 'billykong.github.io/planproof')
        + card(980, 'A DEMO, NOT A PRODUCT', 'full video linked below')
        + t(X, 1220, 'Scripted with AI; AI voice.', 34, C.muted)
        + t(X, 1270, 'All checker output is from real runs.', 34, C.muted));
    }
    return frame(label('THANKS FOR WATCHING')
      + t(X, 260, 'The code, the tests, every result', 72, C.fg, sans, 700)
      + card(340, 'CODE · MIT', 'github.com/billykong/planproof')
      + card(510, 'WRITE-UP', 'billykong.github.io/planproof')
      + t(X, 760, 'Scripted from the write-up with AI and narrated with an AI voice.', 32, C.muted)
      + t(X, 808, 'All checker output is from real runs.', 32, C.muted)
      + stack(1640, 360, 1));
  },
};

// A capture is the real output of a run, trimmed: `#@ name` lines mark
// reveal points and aren't shown; `#title` names the window.
function loadCapture(name) {
  const raw = fs.readFileSync(path.join(__dirname, 'captures', `${name}.txt`), 'utf8').split('\n');
  const lines = [], marks = {};
  let title = name;
  for (const l of raw) {
    if (l.startsWith('#@ ')) marks[l.slice(3).trim()] = lines.length;
    else if (l.startsWith('#title ')) title = l.slice(7);
    else lines.push(l.replace(/\s+$/, ''));
  }
  while (lines.length && !lines[lines.length - 1]) lines.pop();
  marks.end = lines.length;
  return { lines, marks, title };
}

const scenesFor = id => id ? require('./shorts').shorts[id].scenes : require('./scenes').scenes;

function main() {
  if (process.argv[2] === '--dump') { process.stdout.write(JSON.stringify(scenesFor(process.argv[3]))); return; }
  const [out, id] = process.argv.slice(2);
  layout(Boolean(id));
  const { Resvg } = require(require.resolve('@resvg/resvg-js', { paths: [process.cwd(), __dirname] }));
  const timings = JSON.parse(fs.readFileSync(path.join(out, 'timings.json'), 'utf8'));
  fs.mkdirSync(path.join(out, 'frames'), { recursive: true });
  const list = [];
  const render = (svg, width = W) => new Resvg(svg, { font: { loadSystemFonts: true, defaultFontFamily: sans }, fitTo: { mode: 'width', value: width } }).render().asPng();
  const png = (name, svg) => { const f = path.join(out, 'frames', `${name}.png`); fs.writeFileSync(f, render(svg)); return f; };
  for (const sc of scenesFor(id)) {
    const cap = sc.visual.type === 'terminal' ? loadCapture(sc.visual.capture) : null;
    let from = 0;
    sc.beats.forEach((b, i) => {
      const key = `${sc.id}-${i}`, dur = timings[key];
      if (dur === undefined) throw new Error(`no timing for ${key}: run tts.py first`);
      if (!cap) { list.push([png(key, V[sc.visual.type](sc.visual, b)), dur]); return; }
      // Reveal the capture line by line across the beat, the last line
      // landing ~70% through so it can be read while the voice finishes.
      const to = cap.marks[b.show];
      if (to === undefined) throw new Error(`capture ${sc.visual.capture} has no mark ${b.show}`);
      const steps = Math.max(1, to - from), span = dur * 0.7;
      for (let k = 1; k <= steps; k++) {
        const upto = to > from ? from + k : to, d = k < steps ? span / steps : dur - span * (steps - 1) / steps;
        list.push([png(`${key}-${String(k).padStart(3, '0')}`, V.terminal(sc.visual, b, cap, upto)), d]);
      }
      from = to;
    });
  }
  // The concat demuxer needs the last file repeated to honour its duration.
  const txt = list.map(([f, d]) => `file '${f}'\nduration ${d.toFixed(3)}`).join('\n') + `\nfile '${list[list.length - 1][0]}'\n`;
  fs.writeFileSync(path.join(out, 'frames.txt'), txt);
  // The video's thumbnail: the title frame at 1280x720.
  if (!id) fs.writeFileSync(path.join(out, 'thumbnail.png'), render(V.title(), 1280));
  console.log(`${list.length} frames, ${list.reduce((a, [, d]) => a + d, 0).toFixed(1)} s`);
}

if (require.main === module) main();
module.exports = { V, loadCapture, layout };
