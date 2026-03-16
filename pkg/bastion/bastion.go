package bastion

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"time"

	"github.com/unifabric-io/nvair-cli/pkg/logging"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

// BastionExecConfig holds the configuration for executing commands via bastion host
type BastionExecConfig struct {
	BastionUser string
	BastionAddr string
	BastionKey  string

	TargetUser string
	TargetAddr string
	TargetPass string

	Command string
}

// ExecResult holds the result of command execution
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

var (
	startInteractiveSessionFn = startInteractiveSession
	startInteractiveCommandFn = startInteractiveCommand
)

// ExecCommandViaBastion executes a command on target host via bastion host
// It establishes connection through bastion using public key authentication and uses password auth to target
func ExecCommandViaBastion(cfg BastionExecConfig) (*ExecResult, error) {
	bastionClient, err := dialBastion(
		cfg.BastionUser,
		cfg.BastionAddr,
		cfg.BastionKey,
	)
	if err != nil {
		return nil, err
	}
	defer bastionClient.Close()

	conn, err := bastionClient.Dial("tcp", cfg.TargetAddr)
	if err != nil {
		return nil, fmt.Errorf("dial target via bastion failed: %w", err)
	}

	targetClient, err := newTargetClient(
		conn,
		cfg.TargetAddr,
		cfg.TargetUser,
		cfg.TargetPass,
	)
	if err != nil {
		return nil, err
	}
	defer targetClient.Close()

	return execCommand(targetClient, cfg.Command)
}

// ExecCommandOnBastion executes a command directly on bastion host
func ExecCommandOnBastion(cfg BastionExecConfig) (*ExecResult, error) {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return nil, err
	}
	defer bastionClient.Close()
	return execCommand(bastionClient, cfg.Command)
}

// InteractiveSessionViaBastion starts an interactive shell on target host via bastion.
func InteractiveSessionViaBastion(cfg BastionExecConfig) error {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return err
	}
	defer bastionClient.Close()

	conn, err := bastionClient.Dial("tcp", cfg.TargetAddr)
	if err != nil {
		return fmt.Errorf("dial target via bastion failed: %w", err)
	}

	targetClient, err := newTargetClient(
		conn,
		cfg.TargetAddr,
		cfg.TargetUser,
		cfg.TargetPass,
	)
	if err != nil {
		return err
	}
	defer targetClient.Close()

	return startInteractiveSessionFn(targetClient)
}

// InteractiveSessionOnBastion starts an interactive shell directly on bastion host.
func InteractiveSessionOnBastion(cfg BastionExecConfig) error {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return err
	}
	defer bastionClient.Close()
	return startInteractiveSessionFn(bastionClient)
}

// InteractiveCommandViaBastion starts an interactive command on target host via bastion.
func InteractiveCommandViaBastion(cfg BastionExecConfig) error {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return err
	}
	defer bastionClient.Close()

	conn, err := bastionClient.Dial("tcp", cfg.TargetAddr)
	if err != nil {
		return fmt.Errorf("dial target via bastion failed: %w", err)
	}

	targetClient, err := newTargetClient(
		conn,
		cfg.TargetAddr,
		cfg.TargetUser,
		cfg.TargetPass,
	)
	if err != nil {
		return err
	}
	defer targetClient.Close()

	return startInteractiveCommandFn(targetClient, cfg.Command)
}

// InteractiveCommandOnBastion starts an interactive command directly on bastion host.
func InteractiveCommandOnBastion(cfg BastionExecConfig) error {
	bastionClient, err := dialBastion(cfg.BastionUser, cfg.BastionAddr, cfg.BastionKey)
	if err != nil {
		return err
	}
	defer bastionClient.Close()
	return startInteractiveCommandFn(bastionClient, cfg.Command)
}

// newTargetClient creates an SSH client connection to target host through bastion
func newTargetClient(
	conn net.Conn,
	addr, user, pass string,
) (*ssh.Client, error) {

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("new ssh client conn failed: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// dialBastion creates an SSH client connection to bastion host using public key authentication
func dialBastion(user, addr, keyPath string) (*ssh.Client, error) {
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	return ssh.Dial("tcp", addr, cfg)
}

// execCommand executes a command on SSH client and returns the result
func execCommand(client *ssh.Client, cmd string) (*ExecResult, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	err = session.Run(cmd)

	exitCode := 0
	if err != nil {
		// The command execution failed, but the SSH connection was successful.
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return nil, err
		}
	}

	return &ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		ExitCode: exitCode,
	}, nil
}

func startInteractiveSession(client *ssh.Client) error {
	return startInteractive(client, "")
}

func startInteractiveCommand(client *ssh.Client, command string) error {
	return startInteractive(client, command)
}

func startInteractive(client *ssh.Client, command string) error {
	fd := int(os.Stdin.Fd())
	if !terminal.IsTerminal(fd) {
		return fmt.Errorf("interactive session requires a terminal")
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	width, height, err := terminal.GetSize(fd)
	if err != nil {
		width = 80
		height = 24
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return fmt.Errorf("request pty failed: %w", err)
	}

	oldState, err := terminal.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("set terminal raw mode failed: %w", err)
	}
	defer func() {
		_ = terminal.Restore(fd, oldState)
	}()

	session.Stdin = os.Stdin
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr

	if command == "" {
		if err := session.Shell(); err != nil {
			return fmt.Errorf("start interactive shell failed: %w", err)
		}
	} else {
		if err := session.Start(command); err != nil {
			return fmt.Errorf("start interactive command failed: %w", err)
		}
	}

	signalCh := make(chan os.Signal, 8)
	done := make(chan struct{})
	signals := append(forwardSignals(), resizeSignals()...)
	if len(signals) > 0 {
		signal.Notify(signalCh, signals...)
	}

	go func() {
		for {
			select {
			case <-done:
				return
			case sig := <-signalCh:
				if isResizeSignal(sig) {
					w, h, err := terminal.GetSize(fd)
					if err == nil {
						_ = session.WindowChange(h, w)
					}
					continue
				}
				if sshSig, ok := toSSHSignal(sig); ok {
					_ = session.Signal(sshSig)
				}
			}
		}
	}()

	waitErr := session.Wait()
	if len(signals) > 0 {
		signal.Stop(signalCh)
	}
	close(done)
	return waitErr
}

// WaitPingViaBastion waits for target host to be reachable via bastion with ping
func WaitPingViaBastion(ctx context.Context, cfg BastionExecConfig, timeout time.Duration) error {
	host := cfg.TargetAddr
	if h, _, err := net.SplitHostPort(cfg.TargetAddr); err == nil {
		host = h
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pingCfg := cfg
		pingCfg.Command = fmt.Sprintf("ping -c1 -W6 %s", host)
		logging.Info("%s ...", pingCfg.Command)
		res, err := ExecCommandOnBastion(pingCfg)
		if err == nil && res != nil && res.ExitCode == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
