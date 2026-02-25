package ssh

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SwitchPasswordResetConfig holds the configuration for changing a switch password via bastion
// Bastion is accessed with SSH key auth; the switch uses password auth and may force immediate change.
type SwitchPasswordResetConfig struct {
	BastionAddr string
	BastionUser string
	BastionKey  string

	SwitchAddr string
	SwitchUser string

	OldPassword string
	NewPassword string

	Timeout time.Duration
}

// SwitchCopyConfig holds the configuration for copying a file to a switch via bastion
type SwitchCopyConfig struct {
	BastionAddr    string
	BastionUser    string
	BastionKey     string
	SwitchAddr     string
	SwitchUser     string
	SwitchPassword string
	Timeout        time.Duration
}

// ChangeSwitchPasswordViaBastion changes the switch password by SSH'ing through bastion
func ChangeSwitchPasswordViaBastion(cfg SwitchPasswordResetConfig) error {
	changeOnce := func() error {
		signer, err := loadPrivateKeySigner(cfg.BastionKey)
		if err != nil {
			return fmt.Errorf("load private key failed: %w", err)
		}

		clientCfg := &ssh.ClientConfig{
			User: cfg.BastionUser,
			Auth: []ssh.AuthMethod{
				ssh.PublicKeys(signer),
			},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         cfg.Timeout,
		}

		bastionClient, err := ssh.Dial("tcp", cfg.BastionAddr, clientCfg)
		if err != nil {
			return fmt.Errorf("ssh dial failed: %w", err)
		}
		defer bastionClient.Close()

		targetClient, err := dialSwitchThroughBastion(
			bastionClient,
			cfg.SwitchAddr,
			cfg.SwitchUser,
			cfg.OldPassword,
			cfg.Timeout,
		)
		if err != nil {
			return err
		}
		defer targetClient.Close()

		session, err := targetClient.NewSession()
		if err != nil {
			return fmt.Errorf("new switch session failed: %w", err)
		}
		defer session.Close()

		if err := requestPTY(session); err != nil {
			return err
		}

		stdin, stdout, stderr, err := preparePipes(session)
		if err != nil {
			return err
		}

		output := make(chan string, 200)
		go readSwitchPipe(stdout, output)
		go readSwitchPipe(stderr, output)

		if err := session.Shell(); err != nil {
			return fmt.Errorf("start switch shell failed: %w", err)
		}

		if err := handleSwitchPasswordChange(
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

// CopySwitchConfigViaBastion copies a local file to the switch via bastion using scp protocol
func CopySwitchConfigViaBastion(cfg SwitchCopyConfig, localPath, remotePath string) error {
	copyOnce := func() error {
		signer, err := loadPrivateKeySigner(cfg.BastionKey)
		if err != nil {
			return fmt.Errorf("load private key failed: %w", err)
		}

		bastionCfg := &ssh.ClientConfig{
			User:            cfg.BastionUser,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         cfg.Timeout,
		}

		bastionClient, err := ssh.Dial("tcp", cfg.BastionAddr, bastionCfg)
		if err != nil {
			return fmt.Errorf("ssh dial failed: %w", err)
		}
		defer bastionClient.Close()

		targetClient, err := dialSwitchThroughBastion(
			bastionClient,
			cfg.SwitchAddr,
			cfg.SwitchUser,
			cfg.SwitchPassword,
			cfg.Timeout,
		)
		if err != nil {
			return err
		}
		defer targetClient.Close()

		fileInfo, err := os.Stat(localPath)
		if err != nil {
			return fmt.Errorf("stat local file failed: %w", err)
		}

		f, err := os.Open(localPath)
		if err != nil {
			return fmt.Errorf("open local file failed: %w", err)
		}
		defer f.Close()

		session, err := targetClient.NewSession()
		if err != nil {
			return fmt.Errorf("new switch session failed: %w", err)
		}
		defer session.Close()

		stdin, err := session.StdinPipe()
		if err != nil {
			return fmt.Errorf("stdin pipe failed: %w", err)
		}
		stdout, err := session.StdoutPipe()
		if err != nil {
			return fmt.Errorf("stdout pipe failed: %w", err)
		}

		if err := session.Start(fmt.Sprintf("scp -t %s", remotePath)); err != nil {
			return fmt.Errorf("start scp failed: %w", err)
		}

		if err := scpReadResponse(stdout); err != nil {
			return fmt.Errorf("scp ack failed: %w", err)
		}

		header := fmt.Sprintf("C%04o %d %s\n", fileInfo.Mode()&0o777, fileInfo.Size(), "config.yml")
		if _, err := io.WriteString(stdin, header); err != nil {
			return fmt.Errorf("write scp header failed: %w", err)
		}
		if err := scpReadResponse(stdout); err != nil {
			return fmt.Errorf("scp header ack failed: %w", err)
		}

		if _, err := io.Copy(stdin, f); err != nil {
			return fmt.Errorf("copy file data failed: %w", err)
		}
		if _, err := stdin.Write([]byte{0}); err != nil {
			return fmt.Errorf("write scp end failed: %w", err)
		}
		if err := scpReadResponse(stdout); err != nil {
			return fmt.Errorf("scp data ack failed: %w", err)
		}

		if err := stdin.Close(); err != nil {
			return fmt.Errorf("close stdin failed: %w", err)
		}

		return session.Wait()
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}

		if err := copyOnce(); err != nil {
			lastErr = err
			if !shouldRetry(err) {
				return err
			}
			time.Sleep(time.Second)
			continue
		}
		return nil
	}

	return lastErr
}

func shouldRetry(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout() || netErr.Temporary()
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe") || strings.Contains(msg, "ssh dial failed") || strings.Contains(msg, "i/o timeout") || strings.Contains(msg, "connect failed") || strings.Contains(msg, "handshake failed")
}

func handleSwitchPasswordChange(
	stdin io.Writer,
	output <-chan string,
	oldPwd, newPwd string,
	timeout time.Duration,
) error {

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case line := <-output:
			l := strings.TrimSpace(line)

			switch {
			case strings.Contains(l, "Are you sure you want to continue connecting"):
				writeLine(stdin, "yes")

			case strings.Contains(l, "Current password:"):
				writeLine(stdin, oldPwd)

			case strings.Contains(l, "New password:"):
				writeLine(stdin, newPwd)

			case strings.Contains(l, "Retype new password:"):
				writeLine(stdin, newPwd)
				return nil

			case strings.Contains(strings.ToLower(l), "permission denied"):
				return fmt.Errorf("switch login failed: %s", l)

			case strings.Contains(strings.ToLower(l), "password updated successfully"):
				return nil
			}

		case <-timer.C:
			return fmt.Errorf("timeout waiting for switch password prompt")
		}
	}
}

// readSwitchPipe is a switch-specific reader that detects prompts without newlines
func readSwitchPipe(r io.Reader, ch chan<- string) {
	reader := bufio.NewReader(r)
	var buf strings.Builder

	specialPrompts := []string{
		"Are you sure you want to continue connecting (yes/no/[fingerprint])?",
		"Current password:",
		"New password:",
		"Retype new password:",
		"password updated successfully",
	}

	for {
		b, err := reader.ReadByte()
		if err != nil {
			return
		}

		buf.WriteByte(b)
		s := buf.String()

		if b == '\n' {
			line := strings.TrimRight(s, "\r\n")
			ch <- line
			buf.Reset()
			continue
		}

		for _, p := range specialPrompts {
			if strings.HasSuffix(s, p) {
				ch <- s
				buf.Reset()
				break
			}
		}
	}
}

func scpReadResponse(r io.Reader) error {
	var b [1]byte
	if _, err := r.Read(b[:]); err != nil {
		return err
	}
	if b[0] == 0 {
		return nil
	}
	if b[0] == 1 || b[0] == 2 {
		rest, _ := io.ReadAll(r)
		return fmt.Errorf("scp error: %s", strings.TrimSpace(string(rest)))
	}
	return fmt.Errorf("unexpected scp response byte: %d", b[0])
}

// dialSwitchThroughBastion opens a TCP channel via bastion and builds an SSH client to the switch
func dialSwitchThroughBastion(bastion *ssh.Client, addr, user, pass string, timeout time.Duration) (*ssh.Client, error) {
	conn, err := bastion.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial switch via bastion failed: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("new switch ssh conn failed: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}
