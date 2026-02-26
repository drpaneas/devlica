package analyzer

const systemPrompt = `You are an expert developer analyst. You analyze GitHub activity data to extract 
a developer's unique persona - their coding style, values, review patterns, and philosophy.
Be specific and cite concrete examples from the data. Avoid generic statements.
Write in third person about the developer.`

const codeStylePrompt = `Analyze this developer's coding style based on their code samples, commit diffs, and CI/CD configurations.

Developer: %s

CODE SAMPLES:
%s

COMMIT DIFFS:
%s

Extract the following with CONCRETE examples from their code:
1. Naming conventions (variables, functions, types) - show examples
2. Code organization patterns (file structure, module design)
3. Error handling approach (how they handle errors, what patterns they use)
4. Comment style (frequency, tone, what they comment on)
5. Testing patterns (if test files are present - naming, structure, assertion style)
6. Language-specific idioms they prefer
7. Formatting preferences visible in their code
8. Any distinctive patterns that make their code recognizable
9. CI/CD and automation patterns (if workflow files are present)
10. Commit size patterns (do they make small surgical changes or large sweeping ones?)

Be specific. Quote actual code snippets. Do not be generic.`

const reviewStylePrompt = `Analyze this developer's code review style based on their PR review comments.

Developer: %s

PR REVIEW COMMENTS:
%s

Extract the following with CONCRETE examples from their reviews:
1. What do they focus on most? (correctness, style, performance, security, tests, readability)
2. How do they deliver feedback? (direct, diplomatic, questioning, teaching)
3. What recurring themes appear in their reviews?
4. Do they suggest alternatives or just point out problems?
5. How detailed are their reviews? (one-liners vs thorough explanations)
6. What do they praise? What triggers criticism?
7. How do they handle disagreements?

Quote actual review comments as examples. Be specific.`

const communicationPrompt = `Analyze this developer's communication style based on their PR descriptions, issue reports, issue comments, and release notes.

Developer: %s

PULL REQUEST DESCRIPTIONS:
%s

ISSUE COMMENTS:
%s

AUTHORED ISSUES (bug reports, feature requests, proposals):
%s

RELEASE NOTES:
%s

Extract the following:
1. How do they describe problems? (concise vs verbose, structured vs narrative)
2. How do they structure PR descriptions? (bullet points, paragraphs, checklists)
3. Level of technical detail they include
4. Do they reference docs, issues, or other resources?
5. Tone (formal, casual, direct, conversational)
6. How do they explain their reasoning for design decisions?
7. How do they report bugs or request features? (structured, minimal reproduction, detailed context)
8. How do they write release notes? (technical, user-facing, changelog style)

Quote actual excerpts as examples. Be specific.`

const developerIdentityPrompt = `Analyze this developer's identity, interests, and community engagement based on their GitHub profile and activity patterns.

Developer: %s

PROFILE:
%s

STARRED REPOSITORIES (showing their interests):
%s

GISTS:
%s

ORGANIZATIONS:
%s

EXTERNAL CONTRIBUTIONS (PRs to repos they don't own):
%s

RECENT ACTIVITY EVENTS:
%s

Extract the following:
1. What technologies and domains are they most interested in? (based on starred repos and activity)
2. What kind of projects do they build? (tools, libraries, applications, infrastructure)
3. What open-source communities do they participate in?
4. How actively do they contribute to projects they don't own?
5. What is their contribution cadence? (burst vs steady, weekday vs weekend patterns)
6. What organizations are they affiliated with and what does that suggest?
7. What does their profile say about how they want to be perceived professionally?
8. What licensing preferences do they show?

Be specific and data-driven. Avoid speculation without evidence.`

const synthesisPrompt = `You have analyzed a developer's GitHub activity across four dimensions. 
Now synthesize these analyses into a unified developer persona.

Developer: %s

CODE STYLE ANALYSIS:
%s

REVIEW STYLE ANALYSIS:
%s

COMMUNICATION ANALYSIS:
%s

DEVELOPER IDENTITY ANALYSIS:
%s

Respond with a single JSON object (no markdown, no commentary) with these fields:

{
  "coding_philosophy": "What they value most in code and what tradeoffs they consistently make.",
  "code_style_rules": "Concrete, actionable rules that capture how they write code. Format each as an imperative statement.",
  "review_priorities": "Ordered list of what they care about when reviewing code.",
  "review_voice": "How to give feedback in their style. Include example phrasings.",
  "communication_patterns": "How they write PR descriptions, comments, and explanations.",
  "testing_philosophy": "Their approach to testing (if data exists). Write 'No specific testing data was identified.' if none.",
  "distinctive_traits": "What makes this developer unique compared to a generic senior engineer.",
  "developer_interests": "Technologies, domains, and communities they engage with. What topics excite them.",
  "project_patterns": "How they structure projects, what they build, licensing choices, CI/CD preferences.",
  "collaboration_style": "How they interact with the community - issue reporting, mentoring, contributing upstream."
}

All values must be non-empty strings. Be extremely specific. Every statement should be backed
by evidence from the analyses. Use concrete examples and actual phrasings from their GitHub activity.
This persona will be used to make an AI agent emulate this developer, so precision matters.`
