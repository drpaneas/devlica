# devlica

Clone a developer's persona from their GitHub activity into
Cursor AI skills.

Devlica crawls a programmer's entire GitHub history - repos,
commits, pull requests, reviews, issues, starred repos, gists,
and more - then feeds the data to a large language model which
synthesizes the patterns into a structured persona. The persona
is written out as Cursor skill files that guide an AI assistant
to code, review, and communicate the way you do.

## Building

    go build -o devlica .

## Usage

    devlica [flags] <username>

The single required argument is a GitHub username.

Devlica needs two things from its environment: a GitHub token
to read repository data, and an API key for whichever language
model you choose. Set them as environment variables:

    export GITHUB_TOKEN=ghp_...
    export ANTHROPIC_API_KEY=sk-ant-...

Then run it:

    ./devlica drpaneas

The program crawls the user's activity, analyzes the data in
parallel, and writes skill files to `./output`.

## Flags

    -provider string    LLM provider: openai, anthropic, ollama (default "anthropic")
    -model string       LLM model name (default depends on provider)
    -output string      output directory (default "./output")
    -max-repos int      maximum repositories to deep-crawl (default 10)
    -verbose            print debug messages

## Providers

Three providers are supported. OpenAI and Anthropic require
API keys; Ollama runs locally and needs none.

    # Anthropic (default)
    export ANTHROPIC_API_KEY=sk-ant-...
    ./devlica drpaneas

    # OpenAI
    export OPENAI_API_KEY=sk-...
    ./devlica -provider openai drpaneas

    # Ollama (must be running on localhost:11434, or set OLLAMA_HOST)
    ./devlica -provider ollama drpaneas

Default models are claude-sonnet-4-5, gpt-4o, and llama3,
respectively. Use `-model` to override.

## How it works

The program proceeds in four stages.

**Crawl.** It collects the user's full GitHub footprint:
all repositories (not just owned - collaborator and member
repos too), commits with patches sampled across the full
history of each repo, pull requests with diff stats, review
comments with diff context, issue comments, authored issues,
starred repos, gists, organization memberships, releases,
and recent activity events. Repos for deep-crawling are
selected for diversity across languages and time periods
rather than just recency.

**Analyze.** The crawled data is sent to the language model in
four parallel requests: coding style (from code samples and
commit diffs), review style (from review comments), communication
patterns (from PR descriptions, issue reports, and release notes),
and developer identity (from profile, starred repos, gists,
organizations, and external contributions). A fifth request
synthesizes these into a structured persona.

**Benchmark.** If enough review comments with diff context were
found, some are held out for validation. The program generates
simulated reviews from the persona and scores them against the
originals. If the score is low, it refines the persona and
tries again, up to five iterations.

**Generate.** The persona is rendered into three Cursor skill
files using Go templates: coding style, code review, and
developer profile.

## Output

The generated files are plain Markdown with YAML front matter,
suitable for use as Cursor AI skills:

    output/
      <username>-coding-style/SKILL.md
      <username>-code-reviewer/SKILL.md
      <username>-developer-profile/SKILL.md
