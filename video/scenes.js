// The video: narration and what's on screen, one beat at a time. Each beat
// is narrated separately, so the screen changes when the voice moves on.
// Scripted from the blog post (billykong.github.io/planproof/, 2026-10-17);
// keep the two saying the same thing.
//
// Visual types: title, statement, cards, flow, terminal, table, numbers,
// list, end. `show` is how many items are visible during the beat; on a
// terminal beat it's the capture mark to reveal up to, spread over the beat.

module.exports = { scenes: [
  { id: 'title', visual: { type: 'title' }, beats: [
    { say: 'AI agents can now write the plans that change production systems. So who checks the plan?' },
    { say: 'This video is about a small checker, with no AI in it, that checks every plan and explains the result in plain English. It’s a demo of how such a check can work, not a production system. What it is, why you can trust it, and what the evidence says.' },
  ] },

  { id: 'problem', visual: { type: 'statement', label: 'THE PROBLEM', lines: [
    'Change is where outages come from.',
    'AI agents now write the change plans.',
    'Reading every plan doesn’t scale.',
    'Trusting the AI isn’t an answer.',
  ], question: 'A third option: a small checker, with no AI in it.' }, beats: [
    { show: 1, say: 'Change is where outages come from. Google’s SRE book says roughly seventy percent of outages are due to changes in a live system. In 2017, one mistyped input to a routine command took Amazon S3 in its busiest region down for hours.' },
    { show: 2, say: 'And AI agents are getting good at this work. Give one a ticket like “upgrade nodes n1 and n2”, and it writes a step-by-step plan. Most of the time the plan is fine. Sometimes it isn’t, and the mistakes look fine at a glance.' },
    { show: 3, say: 'Reading every plan line by line doesn’t scale, and it’s easy to miss things.' },
    { show: 4, say: 'Trusting the AI isn’t an answer either.' },
    { show: 5, say: 'So I built and tested a third option. It’s called planproof.' },
  ] },

  { id: 'examiner', visual: { type: 'cards', label: 'THE IDEA', title: 'A student and a strict examiner', cards: [
    ['AI', 'The student', 'Brilliant, creative, and sometimes wrong.'],
    ['✓', 'The examiner', 'Checks each line against a fixed rulebook. Small enough to check.'],
  ] }, beats: [
    { show: 1, say: 'Think of a maths student and a strict examiner. The student, the AI, can be brilliant, creative, and sometimes wrong.' },
    { show: 2, say: 'The examiner doesn’t care how the student got the answer. They check each line against a fixed rulebook. You don’t need to trust the student. You need to trust the examiner, and the examiner’s job is small enough to check.' },
    { show: 2, say: 'That’s how proof assistants such as Lean work, and it’s the whole design of planproof.' },
  ] },

  { id: 'flow', visual: { type: 'flow' }, beats: [
    { hl: ['ticket', 'ai'], say: 'One: the AI writes a plan. A goal, like “node n1 drained and cordoned”, and the steps to get there.' },
    { hl: ['checker', 'snapshot', 'policy'], say: 'Two: the checker checks it against a snapshot of the cluster and the team’s policy. If something’s wrong, it says which step breaks which rule, and the AI can fix the plan and try again.' },
    { hl: ['person'], say: 'Three: a person reads a one-page report in plain English. What the plan will do, what was checked, and what was not. They answer one question: is this goal what the ticket asked for?' },
    { hl: ['executor', 'cluster'], say: 'Four: a separate executor runs only proved plans, and stops if the cluster has changed since the check.' },
  ] },

  { id: 'board', visual: { type: 'cards', label: 'HOW THE CHECKER THINKS', title: 'It plays the plan out like a board game', cards: [
    ['1', 'The board', 'A snapshot: nodes, pods, room on each node, each service’s disruption budget.'],
    ['2', 'Each step moves pieces', 'By fixed rules. Moving pods count as down until a wait says they’re back.'],
    ['3', '13 rules after every move', 'Budgets, room on nodes, cordon before drain, the policy’s limits.'],
    ['4', 'The goal at the end', 'The plan must change exactly what its goal says, and nothing else.'],
  ] }, beats: [
    { show: 1, say: 'The checker plays the plan out like a board game. The board is a snapshot of the cluster: which nodes exist, which pods run on them, how much room each node has, and the minimum number of copies each service must keep running. That’s its disruption budget.' },
    { show: 2, say: 'Each step moves pieces by fixed rules. “Drain node n1” moves every pod off it. The checker can’t know where the scheduler will put them, so it assumes the worst: moving pods count as down until a wait step says they’re back.' },
    { show: 3, say: 'After every move, it checks thirteen rules. Did a service drop below its minimum? Is there room? Was the node cordoned before it was drained? Is the plan within the policy’s limits?' },
    { show: 4, say: 'At the end, it compares the result with the goal. The plan must change exactly what its goal says, and nothing else.' },
  ] },

  { id: 'nowait', visual: { type: 'terminal', label: 'A PLAN THAT LOOKS FINE', capture: 'no-wait' }, beats: [
    { show: 'plan', say: 'Here’s a plan for that upgrade ticket. Cordon both nodes, drain both, and wait once at the end, to save time. It looks reasonable.' },
    { show: 'verdict', say: 'Here’s what the checker says. Do not approve. During step four, no cart pods are ready, and checkout drops to one, below its budget of two.' },
    { show: 'verdict', say: 'The problem only shows up in the middle of the plan, which is exactly where people and AI models both tend not to look.' },
  ] },

  { id: 'extra', visual: { type: 'terminal', label: 'A PLAN THAT DOES TOO MUCH', capture: 'extra-change' }, beats: [
    { show: 'plan', say: 'This one is subtler. Every step keeps the rules, but the plan also scales the cart service, which its goal doesn’t mention.' },
    { show: 'verdict', say: 'Refused: a change the goal does not list. That’s what catches an agent that also quietly updated something else.' },
    { show: 'verdict', say: 'And when the checker can’t decide, for example because an autoscaler might change the numbers under it, it says Unknown rather than guessing. It would rather refuse a safe plan than approve an unsafe one.' },
  ] },

  { id: 'trust', visual: { type: 'cards', label: 'WHY TRUST THE CHECKER', title: 'The part that decides is small and plain', cards: [
    ['1', 'Small enough to read', 'About 1,700 lines of Go, with a hard limit of 1,800 checked on every change.'],
    ['2', 'No AI, nothing hidden', 'No network, clock or randomness. Same plan and snapshot, same answer.'],
    ['3', 'Every rule tested both ways', 'A plan it must refuse, and a nearby plan it must accept.'],
    ['4', 'Checked against a real cluster', 'Predicted lowest ready pods, compared with measured.'],
  ] }, beats: [
    { show: 1, say: 'This is the heart of it. The AI is the clever, unpredictable part, so it’s kept outside. The part that decides is small: about seventeen hundred lines of Go, with a hard limit of eighteen hundred, checked on every change. A competent engineer can review all of it in an afternoon.' },
    { show: 2, say: 'There’s no AI in it, and nothing hidden: no network, no clock, no randomness. I re-checked all 247 stored AI runs, and got the identical verdict every time.' },
    { show: 3, say: 'Every rule is tested both ways: a plan it must refuse, and a nearby plan it must accept, so “refused” can’t just mean “broken”.' },
    { show: 4, say: 'And a model of Kubernetes is only useful if it matches Kubernetes. So I ran each kind of step on a real cluster.' },
  ] },

  { id: 'fidelity', visual: { type: 'table', label: 'ON A REAL CLUSTER', title: 'Lowest ready pods: predicted and real',
    cols: ['Step', 'Predicted', 'Real'], widths: [0.5, 0.25, 0.25], rows: [
      ['Restart, new pod first', '4 and 4', '4 and 4'],
      ['Restart, one down at a time', '4 and 3', '4 and 3'],
      ['Scale down', '2 and 4', '2 and 4'],
      ['Scale up', '2 and 4', '2 and 4'],
      ['Cordon, drain, uncordon', '4 and 3', '4 and 3'],
    ], note: 'Two services measured in each run. Raw output: results/kind-test.txt' }, beats: [
    { show: 5, say: 'For each one, I measured the lowest number of ready pods while it happened, for two services at once: a restart two ways, a scale down, a scale up, and a cordon and drain.' },
    { show: 5, say: 'The real cluster never did worse than predicted, and matched the prediction exactly, every time.' },
  ] },

  { id: 'refmon', visual: { type: 'statement', label: 'A 1972 US AIR FORCE STUDY', lines: [
    'Always used.',
    'Tamper-proof.',
    'Simple enough to be analysed and tested.',
  ] }, beats: [
    { show: 3, say: 'The principle is more than fifty years old. A 1972 US Air Force study said the part of a system that enforces security must be always used, tamper-proof, and simple enough to be analysed and tested. planproof applies it to AI.' },
  ] },

  { id: 'goal', visual: { type: 'terminal', label: 'WHAT THE PERSON STILL DOES', capture: 'proved' }, beats: [
    { show: 'proved', say: 'The checker checks a plan against its own goal.' },
    { show: 'notchecked', say: 'It can’t know whether the goal is what the ticket asked for. So every report says so, under “not checked”.' },
    { show: 'notchecked', say: 'That’s deliberate. People tend to over-trust automated checks, experts included, and training doesn’t fix it. So the report states what wasn’t checked as plainly as what was.' },
  ] },

  { id: 'digest', visual: { type: 'statement', label: 'IT HAPPENED IN THE TRIALS', lines: [
    'Asked: roll out an image with a 64-character fingerprint.',
    'Claude Haiku wrote it with 65 characters. Twice.',
    'The plan was consistent, so the checker proved it.',
    'Now a malformed fingerprint is refused outright.',
  ] }, beats: [
    { show: 1, say: 'This isn’t hypothetical. In the trials, Claude Haiku was asked to roll out an image identified by a sixty-four character fingerprint.' },
    { show: 2, say: 'Twice, it copied it with sixty-five characters.' },
    { show: 3, say: 'The plan was internally consistent, so the checker proved it: a safe rollout of an image nobody asked for. The grader caught it, as a careful reader of the goal would.' },
    { show: 4, say: 'I then tightened the checker so a malformed fingerprint is refused outright. On the re-run, the same mistake was stopped.' },
  ] },

  { id: 'agents', visual: { type: 'table', label: 'EVIDENCE 1 · AI MODELS WRITING PLANS', title: '13 tickets: 9 solvable, 4 impossible',
    cols: ['Model', 'Solved', 'Declined', 'Cost, 39 runs'], widths: [0.4, 0.2, 0.2, 0.2], rows: [
      ['Claude Opus 5.5', '27/27', '12/12', '$0.68'],
      ['Claude Sonnet 5.5', '27/27', '12/12', '$0.32'],
      ['Claude Haiku 5.5', '25/27', '12/12', '$0.04'],
      ['Qwen2.5 1.5B (laptop)', '0/9', 'everything', 'free'],
    ], note: 'Claude models: three runs per ticket. Local model: one. Every number from scripts in results/.' }, beats: [
    { show: 0, say: 'Now the evidence. Every number comes from scripts in the repository, and a check fails if a published table drifts from what they produce. Four models wrote plans for thirteen tickets: nine that can be done safely, and four that can’t.' },
    { show: 3, say: 'Opus and Sonnet solved every solvable ticket and declined every impossible one, three runs each. Haiku solved twenty-five of twenty-seven, and its thirty-nine runs cost four cents.' },
    { show: 4, say: 'The small local model wasn’t a fair competitor. It proposed unsafe or unrelated changes on almost every ticket, and the checker refused every one. The safety comes from the checker, not from the model being good.' },
  ] },

  { id: 'seeded', visual: { type: 'numbers', label: 'EVIDENCE 2 · DELIBERATELY BAD PLANS', title: '25 bad plans of the kinds an agent might write', stats: [
    ['20', 'stopped by the checker', 'teal'],
    ['5', 'safe, but not what was asked: the person’s check', 'amber'],
    ['0', 'missed', 'fg'],
  ] }, beats: [
    { show: 1, say: 'Second: twenty-five deliberately bad plans. Draining without cordoning, skipping a wait, touching a protected service, a wrong rollback. The checker stopped twenty.' },
    { show: 3, say: 'The other five it proves, because they’re safe plans that do the wrong thing, like scaling to five when the ticket said four. That’s the one check the report tells the reader to make. None was missed.' },
  ] },

  { id: 'gate', visual: { type: 'statement', label: 'EVIDENCE 3 · COMPARED WITH A POLICY GATE', lines: [
    '13 of the 25: a single-change gate can’t express them.',
    'Where both apply, they agree.',
    'The gate allowed two plans with real problems.',
    'It found a gap in planproof too.',
  ] }, beats: [
    { show: 1, say: 'Third, I compared it with the policy gate from my previous project, safe-ops-agent-k8s, using its own code unchanged. Thirteen of the bad plans drain nodes or change images, which a gate on single changes can’t express at all.' },
    { show: 2, say: 'Where both apply, they agree on the hazards both model.' },
    { show: 3, say: 'The gate allowed two plans with real problems: scaling a service an autoscaler controls, and a scale-down the ticket didn’t ask for.' },
    { show: 4, say: 'And it found a gap in planproof: its policy can’t say “this service may go to zero, that one never”. That’s recorded as a known gap.' },
  ] },

  { id: 'review', visual: { type: 'numbers', label: 'EVIDENCE 4 · DOES THE REPORT HELP A REVIEWER?', title: 'Bad plans rejected by three Claude models', stats: [
    ['12/18', 'reading the raw plan, each model', 'amber'],
    ['18/18', 'reading planproof’s report, each model', 'teal'],
  ], note: 'AI reviewers, not people.' }, beats: [
    { show: 1, say: 'Fourth: does the report help a reviewer? Three Claude models reviewed twelve plans, six good and six bad, three times each. Reading the raw plan, each caught twelve of eighteen bad ones.' },
    { show: 2, say: 'Given planproof’s report, all eighteen.' },
    { show: 3, say: 'Reading raw plans, all three approved the same two bad plans every time: the drain without waiting, and the sixty-five character fingerprint. These are AI reviewers, not people. A study with people would be the real test, and I haven’t run one.' },
  ] },

  { id: 'limits', visual: { type: 'statement', label: 'WHAT IT DOESN’T DO', lines: [
    'It’s a demo, not a production system.',
    'Its model of Kubernetes is tested, not proved.',
    'A person still checks the goal.',
    'Seven operations on nodes and deployments.',
    'The evidence is small.',
  ] }, beats: [
    { show: 1, say: 'What it doesn’t do. First, planproof is a demo of how such a check can work. It isn’t a production system, and it isn’t meant to become one.' },
    { show: 2, say: 'The checker is only as good as its model of Kubernetes. That model matched a real cluster in every test, but it’s tested, not mathematically proved. A proof in Lean is the planned next step.' },
    { show: 3, say: 'A person still checks the goal, by design.' },
    { show: 4, say: 'It covers seven operations: cordon, uncordon, drain, scale, change image, restart and wait.' },
    { show: 5, say: 'And the evidence is small: thirteen tickets, twenty-five bad plans, and AI reviewers rather than people.' },
  ] },

  { id: 'prior', visual: { type: 'list', label: 'BUILT ON DECADES OF PUBLIC WORK', title: 'None of this is new', items: [
    '1969  Hoare logic; the frame problem',
    '1972  The reference monitor',
    '1975  Economy of mechanism',
    '1977  Abstract interpretation',
    '1979  The LCF theorem prover',
    '1997  Proof-carrying code',
    '2004  Plan validation in AI planning',
    '2024  Language models paired with verifiers',
  ] }, beats: [
    { show: 4, say: 'None of this is my invention. Hoare logic and the frame problem date from 1969. The reference monitor, economy of mechanism and abstract interpretation, from the seventies.' },
    { show: 8, say: 'Then the LCF theorem prover, proof-carrying code, plan validation in AI planning, and recent work on pairing language models with verifiers.' },
    { show: 8, say: 'What’s mine is putting those parts together for AI-written ops plans, and measuring the result.' },
  ] },

  { id: 'takeaways', visual: { type: 'list', label: 'THE SHORT VERSION', title: 'What I’d take away', items: [
    'Don’t make the AI trustworthy. Make it not matter.',
    'Check plans, not just single changes.',
    'Make the human’s job small and specific.',
    'Look for prior art first.',
  ] }, beats: [
    { show: 1, say: 'So: don’t make the AI trustworthy. Make it not matter. Put a small, plain checker between the AI and production, and let the AI be as clever as it likes on the other side.' },
    { show: 2, say: 'Check plans, not just single changes. The dangerous moment is often in the middle.' },
    { show: 3, say: 'Make the human’s job small and specific. “Is this goal what we asked for?” is a question a person can answer well. “Is this forty-line plan safe?” isn’t.' },
    { show: 4, say: 'And look for prior art first. Most of the hard thinking was done decades ago.' },
  ] },

  { id: 'end', visual: { type: 'end' }, beats: [
    { say: 'The code, the specification, the tests and every result are on GitHub, and the full write-up is on my blog. Links are in the description.' },
    { say: 'This video was scripted with AI and narrated with an AI voice. The terminal output is from real runs. Thanks for watching.' },
  ] },
] };
