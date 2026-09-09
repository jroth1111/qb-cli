package crowdsniff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/crowdsniff/" + name)
	require.NoError(t, err)
	return data
}

func TestGitHubSource_Discover(t *testing.T) {
	t.Parallel()

	t.Run("no token returns empty result", func(t *testing.T) {
		t.Parallel()
		src := NewGitHubSource(GitHubOptions{Token: "NONE"})
		// Override to empty after construction to simulate no token.
		src.token = ""

		result, err := src.Discover(context.Background(), "notion")
		assert.NoError(t, err)
		assert.Empty(t, result.Endpoints)
		assert.Empty(t, result.BaseURLCandidates)
	})

	t.Run("happy path with fresh repos", func(t *testing.T) {
		t.Parallel()

		searchResp := loadFixture(t, "github-code-search-response.json")
		recentPush := time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/search/code"):
				// Verify text-match accept header.
				assert.Equal(t, "application/vnd.github.text-match+json", r.Header.Get("Accept"))
				assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(searchResp)
			case strings.HasPrefix(r.URL.Path, "/repos/"):
				w.Header().Set("Content-Type", "application/json")
				resp := repoResponse{PushedAt: recentPush}
				_ = json.NewEncoder(w).Encode(resp)
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		src := NewGitHubSource(GitHubOptions{
			BaseURL:       srv.URL,
			Token:         "test-token",
			HTTPClient:    srv.Client(),
			RecencyCutoff: 180 * 24 * time.Hour,
		})

		result, err := src.Discover(context.Background(), "api.notion.com")
		assert.NoError(t, err)
		assert.NotEmpty(t, result.Endpoints)

		// Should have found some endpoints from text matches.
		paths := make(map[string]bool)
		for _, ep := range result.Endpoints {
			paths[ep.Path] = true
			assert.Equal(t, TierCodeSearch, ep.SourceTier)
			assert.Equal(t, "github-code-search", ep.SourceName)
		}

		// At least /v1/users and /v1/projects should be extracted.
		assert.True(t, paths["/v1/users"], "expected /v1/users in results")
		assert.True(t, paths["/v1/projects"], "expected /v1/projects in results")

		// Base URL should include api.notion.com.
		assert.NotEmpty(t, result.BaseURLCandidates)
		assert.Contains(t, result.BaseURLCandidates[0], "api.notion.com")
	})

	t.Run("text matches extract endpoints correctly", func(t *testing.T) {
		t.Parallel()

		recentPush := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/search/code"):
				resp := codeSearchResponse{
					TotalCount: 2,
					Items: []codeSearchItem{
						{
							Name: "client.js",
							Path: "src/client.js",
							Repository: codeSearchRepo{
								FullName: "user/repo1",
							},
							TextMatches: []textMatch{
								{Fragment: "fetch('https://api.example.com/v1/users', { method: 'GET' })"},
								{Fragment: "axios.post('https://api.example.com/v1/projects')"},
							},
						},
						{
							Name: "api.py",
							Path: "src/api.py",
							Repository: codeSearchRepo{
								FullName: "user/repo2",
							},
							TextMatches: []textMatch{
								{Fragment: "requests.get('https://api.example.com/v1/users')"},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			case strings.HasPrefix(r.URL.Path, "/repos/"):
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(repoResponse{PushedAt: recentPush})
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		src := NewGitHubSource(GitHubOptions{
			BaseURL:    srv.URL,
			Token:      "test-token",
			HTTPClient: srv.Client(),
		})

		result, err := src.Discover(context.Background(), "api.example.com")
		assert.NoError(t, err)

		paths := make(map[string]bool)
		for _, ep := range result.Endpoints {
			paths[ep.Path] = true
		}
		assert.True(t, paths["/v1/users"], "expected /v1/users")
		assert.True(t, paths["/v1/projects"], "expected /v1/projects")
	})

	t.Run("stale repos are excluded", func(t *testing.T) {
		t.Parallel()

		stalePush := time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339) // 1 year ago

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/search/code"):
				resp := codeSearchResponse{
					TotalCount: 1,
					Items: []codeSearchItem{
						{
							Name: "old.js",
							Path: "src/old.js",
							Repository: codeSearchRepo{
								FullName: "ancient/repo",
							},
							TextMatches: []textMatch{
								{Fragment: "fetch('https://api.example.com/v1/users')"},
							},
						},
					},
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			case strings.HasPrefix(r.URL.Path, "/repos/"):
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(repoResponse{PushedAt: stalePush})
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		src := NewGitHubSource(GitHubOptions{
			BaseURL:       srv.URL,
			Token:         "test-token",
			HTTPClient:    srv.Client(),
			RecencyCutoff: 180 * 24 * time.Hour,
		})

		result, err := src.Discover(context.Background(), "api.example.com")
		assert.NoError(t, err)
		assert.Empty(t, result.Endpoints, "stale repo endpoints should be excluded")
	})

	t.Run("zero search results returns empty", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/search/code") {
				resp := codeSearchResponse{TotalCount: 0, Items: nil}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
			http.NotFound(w, r)
		}))
		defer srv.Close()

		src := NewGitHubSource(GitHubOptions{
			BaseURL:    srv.URL,
			Token:      "test-token",
			HTTPClient: srv.Client(),
		})

		result, err := src.Discover(context.Background(), "nonexistent.api.com")
		assert.NoError(t, err)
		assert.Empty(t, result.Endpoints)
		assert.Empty(t, result.BaseURLCandidates)
	})

	t.Run("API 5xx returns partial results", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		recentPush := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/search/code"):
				callCount++
				if callCount == 1 {
					// First query succeeds.
					resp := codeSearchResponse{
						TotalCount: 1,
						Items: []codeSearchItem{
							{
								Name: "ok.js",
								Path: "src/ok.js",
								Repository: codeSearchRepo{
									FullName: "good/repo",
								},
								TextMatches: []textMatch{
									{Fragment: "fetch('https://api.example.com/v1/users')"},
								},
							},
						},
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(resp)
				} else {
					// Subsequent queries fail.
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			case strings.HasPrefix(r.URL.Path, "/repos/"):
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(repoResponse{PushedAt: recentPush})
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()

		src := NewGitHubSource(GitHubOptions{
			BaseURL:    srv.URL,
			Token:      "test-token",
			HTTPClient: srv.Client(),
		})

		result, err := src.Discover(context.Background(), "api.example.com")
		assert.NoError(t, err)
		// Should still have results from the first successful request.
		assert.NotEmpty(t, result.Endpoints, "should return partial results on 5xx")
	})
}

func TestBuildSearchQueries(t *testing.T) {
	t.Parallel()

	t.Run("domain-like name uses exact match", func(t *testing.T) {
		t.Parallel()
		queries := buildSearchQueries("api.notion.com")
		assert.Len(t, queries, 2)
		assert.Contains(t, queries[0], `"api.notion.com"`)
		assert.Contains(t, queries[0], "language:javascript")
		assert.Contains(t, queries[1], "language:python")
	})

	t.Run("plain name uses broader query", func(t *testing.T) {
		t.Parallel()
		queries := buildSearchQueries("notion")
		assert.Len(t, queries, 2)
		assert.Contains(t, queries[0], `"notion" api`)
		assert.Contains(t, queries[0], "language:javascript")
	})
}

func TestExtractEndpointsFromTextMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fragment  string
		wantPaths []string
	}{
		{
			name:      "full URL with path",
			fragment:  "fetch('https://api.notion.com/v1/users', { headers })",
			wantPaths: []string{"/v1/users"},
		},
		{
			name:      "multiple URLs in fragment",
			fragment:  "fetch('/v1/users')\nfetch('/v1/projects')",
			wantPaths: []string{"/v1/users", "/v1/projects"},
		},
		{
			name:      "POST method inferred",
			fragment:  "axios.post('https://api.example.com/v1/items')",
			wantPaths: []string{"/v1/items"},
		},
		{
			name:      "GET method inferred",
			fragment:  "requests.get('https://api.example.com/v1/users')",
			wantPaths: []string{"/v1/users"},
		},
		{
			name:      "no path patterns",
			fragment:  "just some random text without API paths",
			wantPaths: nil,
		},
		{
			name:      "path with api prefix",
			fragment:  "client.get('/api/projects/list')",
			wantPaths: []string{"/api/projects/list"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			eps := extractEndpointsFromTextMatches(tt.fragment)
			var paths []string
			for _, ep := range eps {
				paths = append(paths, ep.path)
			}
			if tt.wantPaths == nil {
				assert.Empty(t, paths)
			} else {
				assert.Equal(t, tt.wantPaths, paths)
			}
		})
	}
}

func TestExtractEndpointsMethodInference(t *testing.T) {
	t.Parallel()

	t.Run("POST method from fragment", func(t *testing.T) {
		t.Parallel()
		eps := extractEndpointsFromTextMatches("axios.post('https://api.example.com/v1/items')")
		require.Len(t, eps, 1)
		assert.Equal(t, "POST", eps[0].method)
	})

	t.Run("GET method from fragment", func(t *testing.T) {
		t.Parallel()
		eps := extractEndpointsFromTextMatches("http.Get(\"https://api.example.com/v1/blocks\")")
		require.Len(t, eps, 1)
		assert.Equal(t, "GET", eps[0].method)
	})

	t.Run("no method defaults to empty", func(t *testing.T) {
		t.Parallel()
		eps := extractEndpointsFromTextMatches("fetch('https://api.example.com/v1/data')")
		require.Len(t, eps, 1)
		assert.Equal(t, "", eps[0].method)
	})
}

func TestNewGitHubSource_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("applies defaults", func(t *testing.T) {
		t.Parallel()
		src := NewGitHubSource(GitHubOptions{Token: "tok"})
		assert.Equal(t, "https://api.github.com", src.baseURL)
		assert.NotNil(t, src.client)
		assert.Equal(t, 180*24*time.Hour, src.recencyCutoff)
	})

	t.Run("respects overrides", func(t *testing.T) {
		t.Parallel()
		client := &http.Client{Timeout: 5 * time.Second}
		src := NewGitHubSource(GitHubOptions{
			BaseURL:       "https://custom.api.com",
			HTTPClient:    client,
			Token:         "custom-token",
			RecencyCutoff: 90 * 24 * time.Hour,
		})
		assert.Equal(t, "https://custom.api.com", src.baseURL)
		assert.Equal(t, client, src.client)
		assert.Equal(t, "custom-token", src.token)
		assert.Equal(t, 90*24*time.Hour, src.recencyCutoff)
	})
}
