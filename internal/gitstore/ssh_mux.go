package gitstore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/pmenglund/stacky/internal/config"
)

const sshMuxPersistMinutes = 120

var sshCommandContext = exec.CommandContext

// SSHMux manages ssh ControlMaster sockets used to share connections during git
// operations. It mirrors the Python implementation, which conditionally starts
// a muxed session when share_ssh_session is enabled.
type SSHMux struct {
	repo *Repository
	cfg  config.Config
}

// NewSSHMux constructs an SSHMux bound to the provided repository and
// configuration.
func NewSSHMux(repo *Repository, cfg config.Config) *SSHMux {
	return &SSHMux{repo: repo, cfg: cfg}
}

// EnsureMux starts a shared ssh session for the given remote when configured to
// do so. If the remote does not use ssh or share_ssh_session is disabled, the
// call is a no-op.
func (m *SSHMux) EnsureMux(ctx context.Context, remote string) error {
	if !m.cfg.ShareSSHSession {
		return nil
	}
	host, err := m.remoteHost(ctx, remote)
	if err != nil {
		return err
	}
	if host == "" {
		return nil
	}

	baseArgs := sshBaseArgs()
	commandStr := strings.Join(append([]string{"ssh"}, baseArgs...), " ")
	if err := os.Setenv("GIT_SSH_COMMAND", commandStr); err != nil {
		return fmt.Errorf("gitstore: set GIT_SSH_COMMAND: %w", err)
	}

	args := append(baseArgs, "-MNf", host)
	return runSSH(ctx, "start", host, args)
}

// CloseMux terminates any existing shared ssh session for the remote. Errors
// encountered while stopping the session are returned so callers can surface
// the failure if desired.
func (m *SSHMux) CloseMux(ctx context.Context, remote string) error {
	if !m.cfg.ShareSSHSession {
		return nil
	}
	host, err := m.remoteHost(ctx, remote)
	if err != nil {
		return err
	}
	if host == "" {
		return nil
	}
	args := append(sshBaseArgs(), "-O", "exit", host)
	return runSSH(ctx, "stop", host, args)
}

func (m *SSHMux) remoteHost(ctx context.Context, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	out, err := m.repo.runner.Run(ctx, "git", "remote", "-v")
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] != remote || fields[len(fields)-1] != "(push)" {
			continue
		}
		host := parseSSHHost(fields[1])
		if host != "" {
			return host, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func sshBaseArgs() []string {
	return []string{
		"-o", "ControlMaster=auto",
		"-o", fmt.Sprintf("ControlPersist=%d", sshMuxPersistMinutes),
		"-o", "ControlPath=~/.ssh/stacky-%C",
	}
}

func runSSH(ctx context.Context, action, host string, args []string) error {
	cmd := sshCommandContext(ctx, "ssh", args...)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gitstore: %s ssh mux for %s: %w: %s", action, host, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func parseSSHHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "ssh" && scheme != "git+ssh" {
			return ""
		}
		return u.Host
	}
	idx := strings.Index(raw, ":")
	if idx == -1 {
		return ""
	}
	host := raw[:idx]
	if host == "" || strings.Contains(host, "/") {
		return ""
	}
	return host
}
