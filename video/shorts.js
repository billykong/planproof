// Three Shorts (1080x1920, under a minute each), each one idea from the
// video, with the same captures and the same claims. Same format as
// scenes.js; render.js lays these out vertically.

module.exports = { shorts: {
  'looks-fine': { title: 'This AI plan looks fine. It isn’t.', scenes: [
    { id: 'hook', visual: { type: 'hook', lines: ['This AI plan', 'looks fine.'], kicker: 'It isn’t.' }, beats: [
      { say: 'An AI agent wrote this plan to upgrade two Kubernetes nodes. It looks fine. It isn’t.' },
    ] },
    { id: 'plan', visual: { type: 'terminal', label: 'THE PLAN', capture: 'no-wait' }, beats: [
      { show: 'plan', say: 'Cordon both nodes, drain both, then wait once at the end, to save time.' },
      { show: 'verdict', say: 'A small checker, with no AI in it, plays the plan out step by step. Its verdict: do not approve. During step four, no cart pods are ready, and checkout is below its budget.' },
    ] },
    { id: 'middle', visual: { type: 'statement', label: 'WHY IT’S MISSED', lines: [
      'Every step looks fine.',
      'The middle of the plan isn’t.',
      'Three AI reviewers approved it every time.',
    ] }, beats: [
      { show: 2, say: 'Every single step is fine. The problem only shows up in the middle of the plan, which is exactly where people and AI models both tend not to look.' },
      { show: 3, say: 'Three AI models, reviewing the raw plan, approved it every time. Given the checker’s report, they rejected it.' },
    ] },
    { id: 'end', visual: { type: 'end' }, beats: [
      { say: 'The checker is called planproof: a demo of how such a check can work, not a production system. The full video and the write-up are linked.' },
    ] },
  ] },

  'wrong-goal': { title: 'My checker proved the wrong plan', scenes: [
    { id: 'hook', visual: { type: 'hook', lines: ['My checker', 'proved a plan.'], kicker: 'The wrong plan.', verdict: 'PROVED' }, beats: [
      { say: 'My checker proved that an AI’s plan was safe. It was the wrong plan.' },
    ] },
    { id: 'story', visual: { type: 'statement', label: 'IN THE TRIALS', lines: [
      'Asked: an image with a 64-character fingerprint.',
      'Claude Haiku wrote 65 characters. Twice.',
      'A safe rollout, of an image nobody asked for.',
    ] }, beats: [
      { show: 1, say: 'In the trials, Claude Haiku was asked to roll out an image identified by a sixty-four character fingerprint.' },
      { show: 2, say: 'Twice, it copied it with sixty-five characters.' },
      { show: 3, say: 'The plan was internally consistent, so the checker proved it: a safe rollout of an image nobody asked for.' },
    ] },
    { id: 'notchecked', visual: { type: 'terminal', label: 'EVERY REPORT SAYS', capture: 'proved' }, beats: [
      { show: 'notchecked', say: 'That’s why every report says this. The checker checks a plan against its own goal. Only a person can say whether the goal is what the ticket asked for.' },
    ] },
    { id: 'fix', visual: { type: 'statement', label: 'WHAT CHANGED', lines: [
      'Now a malformed fingerprint is refused outright.',
      'The person’s job: one clear question.',
    ] }, beats: [
      { show: 1, say: 'I tightened the checker, so a malformed fingerprint is now refused outright.' },
      { show: 2, say: 'But the lesson stands: make the person’s job one clear question. Is this goal what we asked for?' },
    ] },
    { id: 'end', visual: { type: 'end' }, beats: [
      { say: 'The checker is called planproof: a demo of how such a check can work, not a production system. The full video and the write-up are linked.' },
    ] },
  ] },

  'reviewers': { title: 'AI reviewers caught 12 of 18 bad plans. Then 18.', scenes: [
    { id: 'hook', visual: { type: 'hook', lines: ['Same bad plans.', 'Same AI reviewers.'], kicker: '12/18 → 18/18' }, beats: [
      { say: 'Three AI models reviewed the same change plans twice. Once as the raw plan, and once with a checker’s report.' },
    ] },
    { id: 'numbers', visual: { type: 'numbers', label: 'BAD PLANS REJECTED', stats: [
      ['12/18', 'reading the raw plan', 'amber'],
      ['18/18', 'reading the checker’s report', 'teal'],
    ] }, beats: [
      { show: 1, say: 'Reading the raw plan, each model caught twelve of eighteen bad plans.' },
      { show: 2, say: 'With the report, all eighteen.' },
    ] },
    { id: 'missed', visual: { type: 'statement', label: 'MISSED EVERY TIME, RAW', lines: [
      'A drain without waiting in between.',
      'A fingerprint one character too long.',
    ] }, beats: [
      { show: 2, say: 'Reading raw plans, all three approved the same two bad plans every time: a drain without waiting in between, and an image fingerprint one character too long.' },
      { show: 2, say: 'Both are things a checker works out mechanically, and a reader has to work out in their head.' },
    ] },
    { id: 'caveat', visual: { type: 'statement', label: 'THE CAVEAT', lines: ['AI reviewers, not people.'] }, beats: [
      { show: 1, say: 'These are AI reviewers, not people. A study with people is the real test.' },
    ] },
    { id: 'end', visual: { type: 'end' }, beats: [
      { say: 'The checker is called planproof: a demo of how such a check can work, not a production system. The full video and the write-up are linked.' },
    ] },
  ] },
} };
