// Package agentapi defines the HTTP protocol between the host server and the
// in-guest sandboxd agent. Both sides (and the CLI) share these types.
package agentapi

import "time"

// Port is the fixed port sandboxd listens on inside the guest. The host
// reaches it directly at guestIP:Port over the bridge (no DNAT involved).
const Port = 8090

// DefaultTimeout bounds command execution when ExecRequest.TimeoutSec is 0.
const DefaultTimeout = 60 * time.Second

// MaxOutputBytes caps captured stdout/stderr per stream.
const MaxOutputBytes = 2 << 20 // 2 MiB

// ExecRequest asks the agent to run a shell command.
type ExecRequest struct {
	Cmd        string            `json:"cmd"`                   // run via bash -lc
	Cwd        string            `json:"cwd,omitempty"`         // default: /home/sandbox/app
	Env        map[string]string `json:"env,omitempty"`         // appended to the agent's env
	TimeoutSec int               `json:"timeout_sec,omitempty"` // default: DefaultTimeout
}

// ExecResult is the outcome of an ExecRequest.
type ExecResult struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out"`
	DurationMS int64  `json:"duration_ms"`
}

// DirEntry is one row of a directory listing.
type DirEntry struct {
	Name  string    `json:"name"`
	Size  int64     `json:"size"`
	Mode  string    `json:"mode"`
	IsDir bool      `json:"is_dir"`
	MTime time.Time `json:"mtime"`
}
