package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Repo struct {
	FullName    string    `json:"full_name"`
	HTMLURL     string    `json:"html_url"`
	Description string    `json:"description"`
	Stars       int       `json:"stargazers_count"`
	Forks       int       `json:"forks_count"`
	Issues      int       `json:"open_issues_count"`
	Language    string    `json:"language"`
	Topics      []string  `json:"topics"`
	CreatedAt   time.Time `json:"created_at"`
	PushedAt    time.Time `json:"pushed_at"`
	License     *struct {
		SPDX string `json:"spdx_id"`
	} `json:"license"`
}

type searchResp struct {
	TotalCount int    `json:"total_count"`
	Items      []Repo `json:"items"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

// apiBase peut être changé via GITHUB_API_URL (GitHub Enterprise, tests).
func apiBase() string {
	if v := os.Getenv("GITHUB_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.github.com"
}

func searchRepos(q, sortBy string, limit int, token string) ([]Repo, error) {
	u := apiBase() + "/search/repositories?" + url.Values{
		"q":        {q},
		"sort":     {sortBy},
		"order":    {"desc"},
		"per_page": {strconv.Itoa(limit)},
	}.Encode()

	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ghwatch")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &e) == nil && e.Message != "" {
			return nil, fmt.Errorf("GitHub %d : %s", resp.StatusCode, e.Message)
		}
		return nil, fmt.Errorf("GitHub %d", resp.StatusCode)
	}
	var r searchResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}
