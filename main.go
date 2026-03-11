package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"

	"github.com/drpaneas/devlica/internal/analyzer"
	"github.com/drpaneas/devlica/internal/benchmark"
	"github.com/drpaneas/devlica/internal/config"
	"github.com/drpaneas/devlica/internal/ghcrawl"
	"github.com/drpaneas/devlica/internal/llm"
	"github.com/drpaneas/devlica/internal/skill"
)

const (
	githubSearchHardCap = 1000
	githubEventsWindow  = 300
)

func main() {
	var cfg config.Config
	var provider string
	configureFlags(flag.CommandLine, &cfg, &provider)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: devlica [flags] <username>\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	cfg.Provider = llm.ProviderName(provider)

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}
	cfg.Username = flag.Arg(0)

	cfg.LoadFromEnv()
	if cfg.Model == "" {
		cfg.Model = config.DefaultModel(cfg.Provider)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, &cfg); err != nil {
		log.Fatal(err)
	}
}

func configureFlags(fs *flag.FlagSet, cfg *config.Config, provider *string) {
	fs.StringVar(provider, "provider", "anthropic", "LLM provider: openai, anthropic, ollama")
	fs.StringVar(&cfg.Model, "model", "", "LLM model (default: per-provider)")
	fs.StringVar(&cfg.OutputDir, "output", "./output", "Output directory for generated skills")
	fs.IntVar(&cfg.MaxRepos, "max-repos", 10, "Maximum repositories to deep-crawl (commits, PRs, code samples)")
	fs.BoolVar(&cfg.Exhaustive, "exhaustive", false, "Crawl exhaustive public GitHub activity data (disables sampling caps)")
	fs.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")
}

func run(ctx context.Context, cfg *config.Config) error {
	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	slog.Info("starting devlica", "username", cfg.Username, "provider", cfg.Provider, "model", cfg.Model)
	if cfg.Provider == llm.ProviderAnthropic {
		authMode := "api_key"
		if cfg.UseVertexAI {
			authMode = "vertex"
		}
		slog.Info("anthropic auth mode",
			"mode", authMode,
		)
	}
	if cfg.Exhaustive {
		slog.Warn("github upstream limits still apply in exhaustive mode",
			"events_limit", githubEventsWindow,
			"search_limit", githubSearchHardCap,
			"note", "GitHub events are limited to recent public activity and Search API is capped per query",
		)
	}

	slog.Info("token pool", "tokens", len(cfg.GitHubTokens), "private_token", cfg.PrivateToken != "")
	crawler := ghcrawl.NewCrawler(cfg.GitHubTokens, cfg.PrivateToken, cfg.MaxRepos, cfg.Exhaustive)
	slog.Info("crawling github activity")
	result, err := crawler.Crawl(ctx, cfg.Username)
	if err != nil {
		return fmt.Errorf("crawling github: %w", err)
	}
	slog.Info("crawl complete",
		"repos", len(result.Repos),
		"commits", result.TotalCommits(),
		"reviews", result.TotalReviews(),
		"issue_comments", len(result.IssueComments),
		"authored_issues", result.TotalIssues(),
		"external_prs", result.TotalExternalPRs(),
		"starred_repos", result.TotalStarred(),
		"gists", result.TotalGists(),
		"releases", result.TotalReleases(),
		"events", len(result.Events),
		"orgs", len(result.Orgs),
		"discussions", result.TotalDiscussions(),
		"projects", result.TotalProjects(),
	)
	logLikelyUpstreamTruncation(result, cfg.Exhaustive)

	heldOut := benchmark.SplitReviews(result, benchmark.MaxHeldOut)
	slog.Info("held out reviews for benchmark", "count", len(heldOut), "remaining_reviews", result.TotalReviews())

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Name:            cfg.Provider,
		APIKey:          cfg.APIKey,
		Model:           cfg.Model,
		OllamaHost:      cfg.OllamaHost,
		UseVertexAI:     cfg.UseVertexAI,
		VertexRegion:    cfg.VertexRegion,
		VertexProjectID: cfg.VertexProjectID,
	})
	if err != nil {
		return fmt.Errorf("creating LLM provider: %w", err)
	}
	a := analyzer.New(provider)
	slog.Info("analyzing developer persona")
	persona, err := a.Analyze(ctx, cfg.Username, result)
	if err != nil {
		return fmt.Errorf("analyzing persona: %w", err)
	}

	if len(heldOut) > 0 {
		bench := benchmark.New(provider)
		slog.Info("benchmarking persona quality")
		benchResult, refined, err := bench.Run(ctx, persona, heldOut)
		if err != nil {
			return fmt.Errorf("benchmarking persona: %w", err)
		}
		persona = refined
		fmt.Fprintf(os.Stderr, "\nBenchmark: score=%.1f/100 iterations=%d\n", benchResult.FinalScore, benchResult.Iterations)
		for _, iter := range benchResult.History {
			fmt.Fprintf(os.Stderr, "  iteration %d: score=%.1f\n", iter.Iteration, iter.Score)
		}
		fmt.Fprintln(os.Stderr)
	} else {
		slog.Warn("no reviews with diff context available, skipping benchmark")
	}

	gen := skill.NewGenerator(cfg.OutputDir)
	slog.Info("generating skill files")
	paths, err := gen.Generate(cfg.Username, persona)
	if err != nil {
		return fmt.Errorf("generating skills: %w", err)
	}

	for _, p := range paths {
		fmt.Println(p)
	}
	slog.Info("done", "skills_generated", len(paths))
	return nil
}

func logLikelyUpstreamTruncation(result *ghcrawl.CrawlResult, exhaustive bool) {
	if !exhaustive {
		return
	}
	if len(result.Events) >= githubEventsWindow {
		slog.Warn("activity events likely truncated by GitHub public events window",
			"events_collected", len(result.Events),
			"window_limit", githubEventsWindow,
		)
	}
	if len(result.AuthoredIssues) >= githubSearchHardCap {
		slog.Warn("authored issues likely truncated by GitHub Search API cap",
			"authored_issues_collected", len(result.AuthoredIssues),
			"search_limit", githubSearchHardCap,
			"query", "author:<user> is:issue",
		)
	}
	if len(result.ExternalPRs) >= githubSearchHardCap {
		slog.Warn("external pull requests likely truncated by GitHub Search API cap",
			"external_prs_collected", len(result.ExternalPRs),
			"search_limit", githubSearchHardCap,
			"query", "author:<user> is:pr -user:<user>",
		)
	}
}
