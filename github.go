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
	Homepage    string    `json:"homepage"`
	Archived    bool      `json:"archived"`
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
	body, status, err := apiGet(u, "application/vnd.github+json", token)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, apiError(status, body)
	}
	var r searchResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	return r.Items, nil
}

const apiMaxBytes = 8 << 20 // garde-fou : une recherche de 100 dépôts pèse moins de 1 Mo

// fetchReadme renvoie un extrait en texte du README d'un dépôt ("" s'il n'en a pas).
func fetchReadme(fullName, token string) (string, error) {
	body, status, err := apiGet(apiBase()+"/repos/"+fullName+"/readme", "application/vnd.github.raw+json", token)
	switch {
	case err != nil:
		return "", err
	case status == http.StatusNotFound:
		return "", nil
	case status != http.StatusOK:
		return "", apiError(status, body)
	}
	return readmeText(string(body), readmeMaxChars), nil
}

func apiGet(u, accept, token string) ([]byte, int, error) {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ghwatch")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, apiMaxBytes))
	return body, resp.StatusCode, err
}

func apiError(status int, body []byte) error {
	var e struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		return fmt.Errorf("GitHub %d : %s", status, e.Message)
	}
	return fmt.Errorf("GitHub %d", status)
}
