# devlica

Clone a developer persona from GitHub activity into Cursor skills.

`devlica` crawls GitHub data (repos, commits, pull requests, reviews, issues, gists, stars, and more), analyzes patterns with an LLM, benchmarks persona quality against held-out review comments, and writes Cursor skill files to disk.

## Current Status

- LLM providers: `anthropic` (default), `openai`, `ollama`
- Anthropic auth modes:
  - API key (`ANTHROPIC_API_KEY`)
  - Google Vertex AI (Claude Code style env vars + ADC)
- Multi-token GitHub crawling supported via `GITHUB_TOKEN`, `GITHUB_TOKEN_1`, `GITHUB_TOKEN_2`, ...
- Optional private token supported via `GITHUB_PRIVATE_TOKEN`
- Exhaustive crawling mode available via `--exhaustive` (subject to GitHub upstream limits)

## Build

```bash
go build -o devlica .
```

## Usage

```bash
./devlica [flags] <github-username>
```

## Required Environment

### GitHub tokens

At least one GitHub token is required:

```bash
export GITHUB_TOKEN=ghp_...
```

Optional extra tokens for pool rotation:

```bash
export GITHUB_TOKEN_1=ghp_...
export GITHUB_TOKEN_2=ghp_...
```

Optional private token:

```bash
export GITHUB_PRIVATE_TOKEN=ghp_...
```

### Anthropic (default provider) with API key

```bash
export ANTHROPIC_API_KEY=sk-ant-...
./devlica drpaneas
```

### Anthropic with Vertex AI (Claude Code style)

`devlica` supports the same Vertex AI style variables used by Claude Code.

```bash
# Enable ADC credentials first (outside devlica)
gcloud auth application-default login
gcloud auth application-default set-quota-project cloudability-it-gemini

# Claude Code style Vertex env
export CLAUDE_CODE_USE_VERTEX=1
export CLOUD_ML_REGION=us-east5
export ANTHROPIC_VERTEX_PROJECT_ID=<your-project-id>

# Run devlica (provider defaults to anthropic)
./devlica drpaneas
```

Notes:

- Vertex mode is enabled only when `CLAUDE_CODE_USE_VERTEX=1`.
- In Vertex mode, both `CLOUD_ML_REGION` and project ID are required.
- Project ID is read from `ANTHROPIC_VERTEX_PROJECT_ID` first, then falls back to `GCLOUD_PROJECT` or `GOOGLE_CLOUD_PROJECT`.
- If Vertex mode is not enabled, `ANTHROPIC_API_KEY` is required for Anthropic.

### OpenAI

```bash
export OPENAI_API_KEY=sk-...
./devlica -provider openai drpaneas
```

### Ollama

```bash
# Optional override, defaults to http://localhost:11434
export OLLAMA_HOST=http://localhost:11434
./devlica -provider ollama drpaneas
```

## Flags

```text
-provider string    LLM provider: openai, anthropic, ollama (default "anthropic")
-model string       LLM model (default: per-provider)
-output string      Output directory for generated skills (default "./output")
-max-repos int      Maximum repositories to deep-crawl (default 10)
-exhaustive         Crawl exhaustive public GitHub activity data (disables sampling caps)
-verbose            Enable verbose logging
```

## Default Models

- `anthropic`: `claude-opus-4-6`
- `openai`: `gpt-4o`
- `ollama`: `llama3`

Use `-model` to override.

## How It Works

1. Crawl GitHub activity and code/review context.
2. Analyze style and behavior in parallel LLM passes.
3. Benchmark persona quality against held-out review comments, and refine when needed.
4. Generate Cursor skill files in the output directory.

## GitHub Upstream Limits

Even in `--exhaustive` mode, some data sources are capped by GitHub:

- Public events are limited to recent activity windows.
- Search API queries are capped per query.

`devlica` logs warnings when collected counts look truncated by those limits.

## Output

Generated skills:

```text
output/
  <username>-coding-style/SKILL.md
  <username>-code-reviewer/SKILL.md
  <username>-developer-profile/SKILL.md
```
