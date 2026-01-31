package seed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	homebrewAnalyticsURL = "https://formulae.brew.sh/api/analytics/install-on-request/30d.json"
	maxResponseBytes     = 10 << 20 // 10 MB
	tier2Threshold       = 40000    // 30-day installs (~10K/week)
	maxRetries           = 3
)

// Tier 1: curated high-impact developer tools.
var tier1Formulas = map[string]bool{
	"ripgrep": true, "fd": true, "bat": true, "eza": true,
	"hyperfine": true, "tokei": true, "delta": true,
	"jq": true, "yq": true, "fzf": true,
	"gh": true, "git-lfs": true,
	"shellcheck": true, "shfmt": true,
	"cmake": true, "ninja": true, "meson": true,
	"go": true, "node": true, "python3": true, "rust": true,
	"kubectl": true, "helm": true, "terraform": true,
	"htop": true, "btop": true, "tmux": true, "tree": true,
	"wget": true, "curl": true,
	"neovim": true, "vim": true,
	"sqlite": true,
}

// HomebrewSource fetches package candidates from Homebrew analytics.
type HomebrewSource struct {
	Client       *http.Client
	AnalyticsURL string // override for testing; defaults to homebrewAnalyticsURL
}

func (s *HomebrewSource) Name() string { return "homebrew" }

type analyticsResponse struct {
	Items []analyticsItem `json:"items"`
}

type analyticsItem struct {
	Formula string `json:"formula"`
	Count   string `json:"count"`
}

func (s *HomebrewSource) Fetch(limit int) ([]Package, error) {
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	analyticsURL := s.AnalyticsURL
	if analyticsURL == "" {
		analyticsURL = homebrewAnalyticsURL
	}

	analytics, err := s.fetchWithRetry(client, analyticsURL)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	items := analytics.Items
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}

	packages := make([]Package, 0, len(items))
	for _, item := range items {
		count := parseCount(item.Count)
		tier := assignTier(item.Formula, count)
		packages = append(packages, Package{
			ID:      "homebrew:" + item.Formula,
			Source:  "homebrew",
			Name:    item.Formula,
			Tier:    tier,
			Status:  "pending",
			AddedAt: now,
		})
	}
	return packages, nil
}

func (s *HomebrewSource) fetchWithRetry(client *http.Client, url string) (*analyticsResponse, error) {
	var lastErr error
	delay := 1 * time.Second

	for attempt := range maxRetries {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = fmt.Errorf("fetch homebrew analytics: %w", err)
			time.Sleep(delay)
			delay *= 2
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("homebrew analytics returned HTTP %d (attempt %d/%d)", resp.StatusCode, attempt+1, maxRetries)
			time.Sleep(delay)
			delay *= 2
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("homebrew analytics returned HTTP %d", resp.StatusCode)
		}

		var analytics analyticsResponse
		dec := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, maxResponseBytes))
		err = dec.Decode(&analytics)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode homebrew analytics: %w", err)
		}
		return &analytics, nil
	}

	return nil, fmt.Errorf("homebrew analytics failed after %d attempts: %w", maxRetries, lastErr)
}

func assignTier(formula string, count int) int {
	if tier1Formulas[formula] {
		return 1
	}
	if count >= tier2Threshold {
		return 2
	}
	return 3
}

func parseCount(s string) int {
	s = strings.ReplaceAll(s, ",", "")
	n, _ := strconv.Atoi(s)
	return n
}
