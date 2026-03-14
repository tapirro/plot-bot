---
name: deep-research
description: Deep research with decomposition, cross-referencing and structured report
argument-hint: "[topic or question]"
context: fork
agent: Explore
allowed-tools: Agent, Bash, Glob, Grep, Read, WebSearch, WebFetch
---

Research the topic: $ARGUMENTS

## Protocol

1. Decompose into 3-5 sub-questions
2. Search each sub-question independently
3. Cross-reference findings across sources
4. Where sources conflict, note it explicitly

## Output Format

### Executive Summary
2-3 sentences.

### Key Findings
- Bulleted list with confidence level (high/medium/low)

### Conflicting Information
- Where sources disagree

### Sources
- Links with dates

### Open Questions
- What remains unclear, suggest next steps
