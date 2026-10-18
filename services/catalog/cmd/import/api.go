package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// maxAttempts is the first try plus the 3 retries of ADR-0012.
	maxAttempts = 4
	// maxErrorBody caps how much of a refusal is kept for the log.
	maxErrorBody = 1024
)

// errUnauthorized means the token is missing, expired or lacks the scope: every later
// write would fail the same way, so the import stops instead of logging 3000 refusals.
var errUnauthorized = errors.New("token refused (expired, or missing the catalog/write scope?)")

// api writes to the catalog API, the only door to the table (ADR-0012).
type api struct {
	http  *http.Client
	base  string
	token string
	// pause is the wait before a retry; a field, not a constant, so tests need not sleep.
	pause time.Duration
}

// put sends body as JSON to PUT base/path..., retrying throttling and server errors.
func (a *api) put(ctx context.Context, body any, path ...string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	// syntaxe : path... déplie la slice en arguments séparés, comme à l'appel de put.
	// pourquoi : JoinPath gère les « / » : peu importe que API_URL finisse par un slash.
	target, err := url.JoinPath(a.base, path...)
	if err != nil {
		return err
	}

	for attempt := 1; ; attempt++ {
		status, msg, err := a.send(ctx, target, data)
		if err == nil && status < 300 {
			return nil
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return fmt.Errorf("PUT %s: %d: %w", target, status, errUnauthorized)
		}

		// pourquoi : on ne reprend que ce qui peut réussir à l'identique plus tard : un
		// pourquoi : 429 (bridé) ou un 5xx. Un 400 est notre validation : il restera un 400.
		retryable := err != nil || status == http.StatusTooManyRequests || status >= 500
		if !retryable || attempt == maxAttempts || ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("PUT %s: %w", target, err)
			}
			return fmt.Errorf("PUT %s: %d %s", target, status, msg)
		}

		// syntaxe : select attend le premier des deux canaux prêts : la pause écoulée, ou
		// syntaxe : l'annulation (Ctrl-C), pour ne pas dormir sur un import interrompu.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(a.pause):
		}
	}
}

// send makes one attempt and returns the status with the start of the response body.
func (a *api) send(ctx context.Context, target string, data []byte) (int, string, error) {
	// pourquoi : un nouveau Reader à chaque tentative : le précédent a été lu jusqu'au bout.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, target, bytes.NewReader(data))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	msg, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil {
		return 0, "", err
	}

	return resp.StatusCode, strings.TrimSpace(string(msg)), nil
}
