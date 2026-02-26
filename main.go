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

func main() {
	var cfg config.Config
	var provider string
	flag.StringVar(&provider, "provider", "anthropic", "LLM provider: openai, anthropic, ollama")
	flag.StringVar(&cfg.Model, "model", "", "LLM model (default: per-provider)")
	flag.StringVar(&cfg.OutputDir, "output", "./output", "Output directory for generated skills")
	flag.IntVar(&cfg.MaxRepos, "max-repos", 10, "Maximum repositories to deep-crawl (commits, PRs, code samples)")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Enable verbose logging")
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

func run(ctx context.Context, cfg *config.Config) error {
	level := slog.LevelInfo
	if cfg.Verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	slog.Info("starting devlica", "username", cfg.Username, "provider", cfg.Provider, "model", cfg.Model)

	crawler := ghcrawl.NewCrawler(cfg.GitHubToken, cfg.MaxRepos)
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
	)

	heldOut := benchmark.SplitReviews(result, benchmark.MaxHeldOut)
	slog.Info("held out reviews for benchmark", "count", len(heldOut), "remaining_reviews", result.TotalReviews())

	provider, err := llm.NewProvider(llm.ProviderConfig{
		Name:       cfg.Provider,
		APIKey:     cfg.APIKey,
		Model:      cfg.Model,
		OllamaHost: cfg.OllamaHost,
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
