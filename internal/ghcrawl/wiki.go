package ghcrawl

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	wikiCloneTimeout = 30 * time.Second
	maxWikiPages     = 20
	maxWikiPageSize  = 32 * 1024
)

// fetchWikiPages clones the wiki repo and reads markdown files.
// Returns nil if the wiki is empty or the clone fails (many repos report
// HasWiki=true even when no wiki exists).
func fetchWikiPages(ctx context.Context, owner, repo, token string) []WikiPage {
	wikiURL := wikiCloneURL(owner, repo)

	tmpDir, err := os.MkdirTemp("", "devlica-wiki-*")
	if err != nil {
		slog.Debug("wiki: could not create temp dir", "error", err)
		return nil
	}
	defer os.RemoveAll(tmpDir)

	cloneCtx, cancel := context.WithTimeout(ctx, wikiCloneTimeout)
	defer cancel()

	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--quiet", wikiURL, tmpDir)
	cmd.Env = gitHubCloneEnv(token)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		slog.Debug("wiki: clone failed (likely no wiki)", "repo", owner+"/"+repo, "error", err)
		return nil
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil
	}

	fullName := owner + "/" + repo
	var pages []WikiPage
	for _, entry := range entries {
		if entry.IsDir() || !isWikiPage(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxWikiPageSize {
			continue
		}
		content, err := os.ReadFile(filepath.Join(tmpDir, entry.Name()))
		if err != nil {
			continue
		}
		title := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		title = strings.ReplaceAll(title, "-", " ")
		pages = append(pages, WikiPage{
			Repo:    fullName,
			Title:   title,
			Content: string(content),
		})
		if len(pages) >= maxWikiPages {
			break
		}
	}
	return pages
}

func wikiCloneURL(owner, repo string) string {
	return fmt.Sprintf("https://github.com/%s/%s.wiki.git", owner, repo)
}

func gitHubCloneEnv(token string) []string {
	env := os.Environ()
	if token == "" {
		return env
	}

	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+auth,
	)
}

func isWikiPage(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".markdown" || ext == ".textile" ||
		ext == ".rdoc" || ext == ".org" || ext == ".creole" ||
		ext == ".mediawiki" || ext == ".rst" || ext == ".asciidoc" ||
		ext == ".pod"
}
