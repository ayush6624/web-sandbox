package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ayush6624/web-sandbox/internal/agentapi"
)

// agentClient talks to in-guest sandboxd agents. No overall timeout — exec
// requests are bounded by their own timeout_sec and the request context.
var agentClient = &http.Client{}

// handleAgentProxy forwards a request to the sandbox's in-guest agent,
// rewriting /sandboxes/{id}/<endpoint> to http://guestIP:agentPort/<endpoint>.
func (s *Server) handleAgentProxy(endpoint string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sb, err := s.reg.Get(r.Context(), id)
		if err != nil {
			httpError(w, 404, err)
			return
		}

		url := fmt.Sprintf("http://%s:%d/%s", sb.GuestIP, agentapi.Port, endpoint)
		if r.URL.RawQuery != "" {
			url += "?" + r.URL.RawQuery
		}
		req, err := http.NewRequestWithContext(r.Context(), r.Method, url, r.Body)
		if err != nil {
			httpError(w, 500, err)
			return
		}
		req.Header.Set("Content-Type", r.Header.Get("Content-Type"))

		resp, err := agentClient.Do(req)
		if err != nil {
			httpError(w, 502, fmt.Errorf("agent unreachable: %w", err))
			return
		}
		defer resp.Body.Close()

		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}
}

// waitForAgent polls the guest agent's /health until it responds or the
// deadline passes. A fresh VM needs a few seconds for systemd to bring the
// network and sandboxd up.
func waitForAgent(ctx context.Context, guestIP string, deadline time.Duration) error {
	url := fmt.Sprintf("http://%s:%d/health", guestIP, agentapi.Port)
	probe := &http.Client{Timeout: 1 * time.Second}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := probe.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("agent not ready after %s: %w", deadline, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}
