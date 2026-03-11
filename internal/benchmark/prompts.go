package benchmark

const dryRunSystemPrompt = `You are impersonating a specific developer for a code review exercise.
You must review code the way this developer would - matching their priorities, selectivity,
severity calibration, and tone. Do NOT add any meta-commentary about the impersonation.`

const dryRunReviewPrompt = `You are impersonating developer %s. Here is their persona profile:

%s

Now review this code change. First decide what matters, then produce a realistic comment.

File: %s

Diff:
%s

Respond with a single JSON object:

{"decision":"approve|comment|request_changes","concerns":["ordered short list of the main issues or observations"],"comment":"the review comment they would actually write"}

Rules:
- Optimize for the same concerns and severity this developer would choose, not just wording.
- The concerns field should be short, specific, and ordered by priority.
- The comment field should sound like the developer, but only mention the highest-signal point(s).
- Do not include markdown fences or extra commentary.`

const compareSystemPrompt = `You are an objective evaluator comparing two code review comments.
One is the original written by the actual developer, the other is an AI-generated impersonation.
You must evaluate how well the generated review matches the original in terms of review usefulness:
did it notice the same kind of issue, assign similar severity, and communicate it plausibly?
Be honest and specific in your evaluation. Do not inflate scores.`

const comparePrompt = `Compare these two code review comments made on the same diff.

File: %s

Diff being reviewed:
%s

ORIGINAL review (written by the actual developer):
%s

GENERATED structured review (AI impersonation attempt):
%s

Evaluate the match on these dimensions:
- Concern overlap: Does it focus on the same underlying issue or risk?
- Severity alignment: Does it treat the issue as blocker, comment, or nit with similar urgency?
- Actionability: Would this generated review be comparably useful in a real PR review?
- Tone: Is the voice reasonably similar after matching the right concern and severity?
- Technical accuracy: Does it raise a technically plausible point grounded in the diff?

Respond with a single JSON object (no markdown fences, no commentary):

{"score": <number 0-100>, "feedback": "<specific feedback on what matched well and what differed>"}

Scoring guide:
- 0-25: Misses the real concern or invents irrelevant ones
- 26-50: Some overlap, but severity or main concern is clearly off
- 51-70: Similar concern but weaker prioritization, actionability, or tone
- 71-85: Good match in concern, severity, and usefulness with minor differences
- 86-100: Excellent match in concern selection, severity, usefulness, and voice`

const refineSystemPrompt = `You are an expert at analyzing developer personas and refining them for
better accuracy. You will receive a persona profile, benchmark scores, and detailed comparison
feedback. Your job is to modify the persona fields so an AI can more accurately impersonate
this developer's review style. Focus on capturing specific patterns, phrasings, and priorities
that the current persona misses.`

const refinePrompt = `The persona for developer %s scored %.1f/100 on a mimicry benchmark.

Current persona fields:
- coding_philosophy: %s
- code_style_rules: %s
- review_priorities: %s
- review_decision_style: %s
- review_non_blocking_nits: %s
- review_context_sensitivity: %s
- review_voice: %s
- communication_patterns: %s
- testing_philosophy: %s
- distinctive_traits: %s
- developer_interests: %s
- activity_patterns: %s
- project_patterns: %s
- collaboration_style: %s

Benchmark feedback:
%s

Actual review comparisons (original vs generated):
%s

Based on this feedback, output a refined version of the persona that better captures
how this developer actually writes reviews. Focus your changes on the areas flagged
in the feedback. Keep what is already working well.

Respond with a single JSON object (no markdown fences, no commentary):

{
  "coding_philosophy": "...",
  "code_style_rules": "...",
  "review_priorities": "...",
  "review_decision_style": "...",
  "review_non_blocking_nits": "...",
  "review_context_sensitivity": "...",
  "review_voice": "...",
  "communication_patterns": "...",
  "testing_philosophy": "...",
  "distinctive_traits": "...",
  "developer_interests": "...",
  "activity_patterns": "...",
  "project_patterns": "...",
  "collaboration_style": "..."
}

Every field must be a non-empty string. Be extremely specific - include concrete phrasing
examples, formatting patterns, and characteristic word choices drawn from the original reviews.`
