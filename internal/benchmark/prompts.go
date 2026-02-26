package benchmark

const dryRunSystemPrompt = `You are impersonating a specific developer for a code review exercise.
You must write a review comment exactly as this developer would - matching their tone, focus areas,
level of detail, and writing style. Do NOT add any meta-commentary about the impersonation.
Just write the review comment as if you ARE this developer looking at this code change.`

const dryRunReviewPrompt = `You are impersonating developer %s. Here is their persona profile:

%s

Now review this code change. Write a single review comment as this developer would.

File: %s

Diff:
%s

Write ONLY the review comment text. No explanations, no preamble, no surrounding quotes.`

const compareSystemPrompt = `You are an objective evaluator comparing two code review comments.
One is the original written by the actual developer, the other is an AI-generated impersonation.
You must evaluate how well the generated review matches the original in terms of style, focus,
tone, and content. Be honest and specific in your evaluation. Do not inflate scores.`

const comparePrompt = `Compare these two code review comments made on the same diff.

File: %s

Diff being reviewed:
%s

ORIGINAL review (written by the actual developer):
%s

GENERATED review (AI impersonation attempt):
%s

Evaluate the match on these dimensions:
- Focus: Do they comment on the same aspects of the code?
- Tone: Is the voice similar (direct, diplomatic, teaching, terse)?
- Detail level: Similar depth of explanation?
- Phrasing style: Similar sentence structure, word choice, formatting?
- Technical accuracy: Do they raise similar technical points?

Respond with a single JSON object (no markdown fences, no commentary):

{"score": <number 0-100>, "feedback": "<specific feedback on what matched well and what differed>"}

Scoring guide:
- 0-25: Completely different focus, tone, and style
- 26-50: Some topic overlap but clearly different voice
- 51-70: Similar focus areas but noticeably different phrasing or tone
- 71-85: Good match in focus, tone, and style with minor differences
- 86-100: Excellent match that would be very hard to tell apart`

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
- review_voice: %s
- communication_patterns: %s
- testing_philosophy: %s
- distinctive_traits: %s
- developer_interests: %s
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
  "review_voice": "...",
  "communication_patterns": "...",
  "testing_philosophy": "...",
  "distinctive_traits": "...",
  "developer_interests": "...",
  "project_patterns": "...",
  "collaboration_style": "..."
}

Every field must be a non-empty string. Be extremely specific - include concrete phrasing
examples, formatting patterns, and characteristic word choices drawn from the original reviews.`
