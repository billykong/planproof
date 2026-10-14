# R - The review packet

**Goal:** a person understands, in about a minute, what a plan changes, what the kernel checked and what it did not, and trusts the result for the right reasons. Covers goal G4 and part of G6.

Status: **Planned**. Spec: [06](../../spec/06-review-packet.md), [glossary](../../spec/glossary.md).

## Why a controlled language

The kernel exists to make a person confident about an agent's work. A correct verdict that a person misreads is worth nothing; a verdict that sounds more certain than it is does harm. ASD-STE100 (Simplified Technical English) was made for the same problem in aircraft maintenance: instructions that must be understood correctly, first time, by people under time pressure, many of them not native English speakers. The packet borrows its principles: a fixed word list with one meaning per word, short sentences, active voice, one instruction per sentence, and standard CAUTION and NOTE blocks.

## User stories

- As an **approver**, I read the goal and the result first, and I know exactly what the kernel did not check.
- As an **approver who doesn't speak English as a first language**, every sentence is short and uses words with one meaning.
- As a **security reviewer**, I can confirm that nothing in the packet was written by a model, and that nothing the kernel found is left out.

## Requirements

| ID | Requirement | Priority | Status |
| --- | --- | --- | --- |
| R-01 | The renderer is a pure function from verdict JSON to text, with fixed templates; no model writes any part of the packet. | Must | Planned |
| R-02 | The packet has the fixed structure in spec 06, with "Not checked" always present and always starting with the statement that the goal itself is not checked. | Must | Planned |
| R-03 | Completeness: every fact produces at least one sentence and every sentence comes from a fact. Tested with a debug mode that tags each sentence with its fact. | Must | Planned |
| R-04 | "Proved" is always worded as "no violation of the N rules in section 3"; the words in W9 (including "safe") never appear. | Must | Planned |
| R-05 | A template linter in CI checks W2, W4, W5, W9 and a passive-voice heuristic on every template filled with sample values. | Must | Planned |
| R-06 | The project's word list and glossary are versioned files; a new word needs a review like code. | Must | Planned |
| R-07 | Free text from the plan appears only quoted, in the unchecked section, with control characters removed and length capped. | Must | Planned |
| R-08 | Golden packet tests for every example, compared byte for byte. | Must | Planned |
| R-09 | A Markdown output for pull requests and chat, with the same sentences. | Should | Planned |
| R-10 | The ASD-STE100 specification's dictionary is not copied into the repo; the word list is the project's own. | Must | Planned |
| R-11 | A readability check: the packet's sentences score as plain English on a standard readability measure, recorded per example. | Could | Planned |
