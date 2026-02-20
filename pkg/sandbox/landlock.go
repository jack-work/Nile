// Package sandbox applies Landlock filesystem restrictions to the neb process.
package sandbox

import (
	"fmt"
	"os"
	"os/exec"
)

// Config holds sandbox configuration for a neb process.
type Config struct {
	// NebBinary is the path to the neb executable.
	NebBinary string

	// StateDir is the neb's read-write state directory.
	StateDir string

	// RetainDir is the directory for retain snapshots (read-write).
	RetainDir string

	// ExtraReadPaths are additional read-only paths.
	ExtraReadPaths []string

	// ExtraWritePaths are additional read-write paths.
	ExtraWritePaths []string

	// Network allows the neb to use network sockets (for Phase 2 inter-copt).
	Network bool
}

// Command creates an exec.Cmd for the neb with sandbox-appropriate setup.
// On Linux with Landlock support, restrictions are applied.
// The command inherits stdin/stdout for JSON-RPC and stderr for logging.
//
// Note: Full Landlock enforcement requires the go-landlock library and
// a two-stage exec (fork + restrict + exec). For now we set up the
// command with restricted environment and rely on systemd's sandboxing
// for additional protection. Full Landlock integration is a follow-up.
func Command(cfg Config) (*exec.Cmd, error) {
	if _, err := os.Stat(cfg.NebBinary); err != nil {
		return nil, fmt.Errorf("sandbox: neb binary not found: %w", err)
	}

	cmd := exec.Command(cfg.NebBinary)

	// Minimal environment
	env := []string{
		"PATH=/usr/bin:/bin",
		"NILE_STATE_DIR=" + cfg.StateDir,
		"NILE_RETAIN_DIR=" + cfg.RetainDir,
	}
	if cfg.Network {
		env = append(env, "NILE_NETWORK=1")
	}
	cmd.Env = env

	return cmd, nil
}
