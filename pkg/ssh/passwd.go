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

// ChangeRemotePassword changes a user's password on a remote host via SSH
func ChangeRemotePassword(cfg ChangePasswordConfig) error {
	signer, err := loadPrivateKeySigner(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("load private key failed: %w", err)
	}

	clientCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}

	client, err := ssh.Dial("tcp", cfg.Addr, clientCfg)
	if err != nil {
		return fmt.Errorf("ssh dial failed: %w", err)
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

	err = handlePasswordChange(
		stdin,
		output,
		cfg.OldPassword,
		cfg.NewPassword,
		cfg.Timeout,
	)
	if err != nil {
		return err
	}

	_ = session.Wait()
	return nil
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

		// 普通换行
		if b == '\n' {
			line := strings.TrimRight(s, "\r\n")
			ch <- line
			buf.Reset()
			continue
		}

		// passwd 的特殊 prompt（无换行）
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
