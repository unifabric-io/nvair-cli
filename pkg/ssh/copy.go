package ssh

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

// BastionCopyConfig holds the configuration for copying a file to a target host via bastion.
type BastionCopyConfig struct {
	BastionAddr string
	BastionUser string
	BastionKey  string

	TargetAddr string
	TargetUser string
	TargetPass string

	Timeout time.Duration
}

// CopyFileViaBastion copies a local file to the target host through bastion using SCP.
func CopyFileViaBastion(cfg BastionCopyConfig, localPath, remotePath string) error {
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
		return fmt.Errorf("ssh dial bastion failed: %w", err)
	}
	defer bastionClient.Close()

	targetClient, err := dialHostThroughBastion(bastionClient, cfg.TargetAddr, cfg.TargetUser, cfg.TargetPass, cfg.Timeout)
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
		return fmt.Errorf("new session failed: %w", err)
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

	header := fmt.Sprintf("C%04o %d %s\n", fileInfo.Mode()&0o777, fileInfo.Size(), filepath.Base(remotePath))
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

// dialHostThroughBastion opens a TCP channel via bastion and builds an SSH client to the target host.
func dialHostThroughBastion(bastion *ssh.Client, addr, user, pass string, timeout time.Duration) (*ssh.Client, error) {
	conn, err := bastion.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial target via bastion failed: %w", err)
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
		return nil, fmt.Errorf("new ssh conn to target failed: %w", err)
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}
