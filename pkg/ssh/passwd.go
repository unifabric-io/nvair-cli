package ssh

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// ChangePasswordConfig holds the configuration for changing a remote password via SSH
type ChangePasswordConfig struct {
	Addr       string
	User       string
	PrivateKey string

	OldPassword string
	NewPassword string

	Timeout time.Duration
}

// WaitForSSHConfig holds the configuration for waiting until SSH public key authentication is ready.
type WaitForSSHConfig struct {
	Addr           string
	User           string
	PrivateKey     string
	ConnectTimeout time.Duration
	ReadyTimeout   time.Duration
	PollInterval   time.Duration
}

// WaitForPublicKeyAuth waits until a remote SSH server accepts public key authentication.
func WaitForPublicKeyAuth(cfg WaitForSSHConfig) error {
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 10 * time.Second
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = 60 * time.Second
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}

	deadline := time.Now().Add(cfg.ReadyTimeout)
	var lastErr error

	for {
		if err := dialWithPublicKey(cfg.Addr, cfg.User, cfg.PrivateKey, cfg.ConnectTimeout); err == nil {
			return nil
		} else {
			lastErr = err
			if strings.Contains(strings.ToLower(err.Error()), "load private key failed") {
				return err
			}
		}

		if time.Now().After(deadline) {
			break
		}

		time.Sleep(cfg.PollInterval)
	}

	return fmt.Errorf("ssh public key authentication not ready after %v: %w", cfg.ReadyTimeout, lastErr)
}

// ChangeRemotePassword changes a user's password on a remote host via SSH
func ChangeRemotePassword(cfg ChangePasswordConfig) error {
	changeOnce := func() error {
		client, err := dialWithPublicKeyClient(cfg.Addr, cfg.User, cfg.PrivateKey, cfg.Timeout)
		if err != nil {
			return err
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			return fmt.Errorf("new session failed: %w", err)
		}
		defer session.Close()

		if err := requestPTY(session); err != nil {
			return err
		}

		stdin, stdout, stderr, err := preparePipes(session)
		if err != nil {
			return err
		}

		output := make(chan string, 100)
		go readPipe(stdout, output)
		go readPipe(stderr, output)

		if err := session.Shell(); err != nil {
			return fmt.Errorf("start shell failed: %w", err)
		}

		if err := handlePasswordChange(
			stdin,
			output,
			cfg.OldPassword,
			cfg.NewPassword,
			cfg.Timeout,
		); err != nil {
			return err
		}

		_ = session.Wait()
		return nil
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}

		if err := changeOnce(); err != nil {
			lastErr = err
			if !shouldRetry(err) {
				return err
			}
			continue
		}
		return nil
	}

	return lastErr
}

func dialWithPublicKey(addr, user, privateKey string, timeout time.Duration) error {
	client, err := dialWithPublicKeyClient(addr, user, privateKey, timeout)
	if err != nil {
		return err
	}
	defer client.Close()
	return nil
}

func dialWithPublicKeyClient(addr, user, privateKey string, timeout time.Duration) (*ssh.Client, error) {
	signer, err := loadPrivateKeySigner(privateKey)
	if err != nil {
		return nil, fmt.Errorf("load private key failed: %w", err)
	}

	clientCfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial failed: %w", err)
	}

	return client, nil
}

func loadPrivateKeySigner(path string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(keyBytes)
}

func requestPTY(session *ssh.Session) error {
	modes := ssh.TerminalModes{
		ssh.ECHO:          0, // 密码不回显
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	return session.RequestPty("xterm", 80, 40, modes)
}

func preparePipes(session *ssh.Session) (io.Writer, io.Reader, io.Reader, error) {
	stdin, err := session.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	return stdin, stdout, stderr, nil
}

func readPipe(r io.Reader, ch chan<- string) {
	reader := bufio.NewReader(r)
	var buf strings.Builder

	specialPrompts := []string{
		"Current password:",
		"New password:",
		"Retype new password:",
	}

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}

		buf.WriteByte(b)
		s := buf.String()

		// normal line with newline
		if b == '\n' {
			line := strings.TrimRight(s, "\r\n")
			ch <- line
			buf.Reset()
			continue
		}

		// special prompts for passwd (no newline)
		for _, p := range specialPrompts {
			if strings.HasSuffix(s, p) {
				ch <- s
				buf.Reset()
				break
			}
		}
	}
}

func handlePasswordChange(
	stdin io.Writer,
	output <-chan string,
	oldPwd, newPwd string,
	timeout time.Duration,
) error {

	state := "init"
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case line := <-output:
			l := strings.TrimSpace(line)

			switch {

			case strings.Contains(l, "Changing password for"):
				writeLine(stdin, oldPwd)
				state = "old"

			case strings.Contains(l, "Current password:"):
				writeLine(stdin, oldPwd)
				state = "old"

			case strings.Contains(l, "New password:") && state == "old":
				writeLine(stdin, newPwd)
				state = "new"

			case strings.Contains(l, "Retype new password:") && state == "new":
				writeLine(stdin, newPwd)
				return nil
			}

		case <-timer.C:
			return fmt.Errorf("timeout waiting for password prompt")
		}
	}
}

func writeLine(w io.Writer, s string) {
	var buf bytes.Buffer
	buf.WriteString(s)
	buf.WriteByte('\n')
	_, _ = w.Write(buf.Bytes())
}
