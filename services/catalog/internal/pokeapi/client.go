// Package pokeapi reads PokéAPI, caching every response on disk so that a rerun
// never downloads the same document twice (ADR-0012).
package pokeapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const (
	baseURL   = "https://pokeapi.co/api/v2/"
	userAgent = "monster-arena-import (+https://github.com/Xtazhoxton/monster-arena)"
)

// Client downloads PokéAPI documents through an on-disk cache.
type Client struct {
	http     *http.Client
	cacheDir string
}

// New returns a Client caching in cacheDir, which it creates if needed.
func New(cacheDir string) (*Client, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create cache directory: %w", err)
	}

	return &Client{
		http:     &http.Client{Timeout: 30 * time.Second},
		cacheDir: cacheDir,
	}, nil
}

// get decodes into out the JSON document at path, relative to the API root
// ("type/fire", "move?limit=10000"), from the cache when present.
func (c *Client) get(ctx context.Context, path string, out any) error {
	body, err := c.fetch(ctx, path)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	return nil
}

// writeAtomic writes data to file through a temporary file and a rename, so that
// file is either absent or complete — never truncated, even if the process dies.
func writeAtomic(file string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), "*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), file)
}

// fetch returns the raw document at path: from the cache file if it exists,
// otherwise downloaded and then cached.
func (c *Client) fetch(ctx context.Context, path string) ([]byte, error) {
	file := filepath.Join(c.cacheDir, url.QueryEscape(path)+".json")

	body, err := os.ReadFile(file)
	if err == nil {
		return body, nil
	}

	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read cache for %s: %w", path, err)
	}
	body, err = c.download(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(file, body); err != nil {
		return nil, fmt.Errorf("cache %s: %w", path, err)
	}
	return body, nil
}

// download performs GET baseURL+path and returns the body of a 200 response.
func (c *Client) download(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body of %s: %w", path, err)
	}
	return body, nil
}
