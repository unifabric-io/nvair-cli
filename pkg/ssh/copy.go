package ssh

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	// DirectTarget indicates the target is the bastion host itself and no second SSH hop is needed.
	DirectTarget bool
}

// CopyFileViaBastion copies a local file to the target host through bastion using SCP.
func CopyFileViaBastion(cfg BastionCopyConfig, localPath, remotePath string) error {
	copyOnce := func() error {
		targetClient, cleanup, err := dialTargetClient(cfg)
		if err != nil {
			return err
		}
		defer cleanup()

		fileInfo, err := os.Stat(localPath)
		if err != nil {
			return fmt.Errorf("stat local file failed: %w", err)
		}
		if fileInfo.IsDir() {
			return fmt.Errorf("copying directories is not supported: %s", localPath)
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

		if err := session.Start(fmt.Sprintf("scp -t %s", scpShellQuote(remotePath))); err != nil {
			return fmt.Errorf("start scp failed: %w", err)
		}

		if err := scpReadResponse(stdout); err != nil {
			return fmt.Errorf("scp ack failed: %w", err)
		}

		targetName := filepath.Base(remotePath)
		if targetName == "." || targetName == "/" || strings.HasSuffix(remotePath, "/") {
			targetName = filepath.Base(localPath)
		}
		header := fmt.Sprintf("C%04o %d %s\n", fileInfo.Mode()&0o777, fileInfo.Size(), targetName)
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

	return withCopyRetry(copyOnce)
}

// CopyFileFromBastion copies a remote file from the target host through bastion to local path using SCP.
func CopyFileFromBastion(cfg BastionCopyConfig, remotePath, localPath string) error {
	copyOnce := func() error {
		targetClient, cleanup, err := dialTargetClient(cfg)
		if err != nil {
			return err
		}
		defer cleanup()

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

		if err := session.Start(fmt.Sprintf("scp -f %s", scpShellQuote(remotePath))); err != nil {
			return fmt.Errorf("start scp failed: %w", err)
		}

		reader := bufio.NewReader(stdout)
		if _, err := stdin.Write([]byte{0}); err != nil {
			return fmt.Errorf("write scp start ack failed: %w", err)
		}

		mode, size, remoteName, err := scpReadFileHeader(reader)
		if err != nil {
			return err
		}

		// SCP protocol requires an ACK after receiving the file header.
		// Without this, sender may wait and never stream file data.
		if _, err := stdin.Write([]byte{0}); err != nil {
			return fmt.Errorf("write scp header ack failed: %w", err)
		}

		destinationPath, err := resolveLocalDestination(localPath, remoteName)
		if err != nil {
			return err
		}

		parentDir := filepath.Dir(destinationPath)
		if parentDir != "." && parentDir != "" {
			if err := os.MkdirAll(parentDir, 0o755); err != nil {
				return fmt.Errorf("create local directory failed: %w", err)
			}
		}

		f, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return fmt.Errorf("open local destination failed: %w", err)
		}

		if _, err := io.CopyN(f, reader, size); err != nil {
			_ = f.Close()
			return fmt.Errorf("copy file data failed: %w", err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("close local file failed: %w", err)
		}

		if err := scpReadResponse(reader); err != nil {
			return fmt.Errorf("scp data ack failed: %w", err)
		}
		if _, err := stdin.Write([]byte{0}); err != nil {
			return fmt.Errorf("write scp final ack failed: %w", err)
		}
		if err := stdin.Close(); err != nil {
			return fmt.Errorf("close stdin failed: %w", err)
		}

		if err := session.Wait(); err != nil {
			return fmt.Errorf("wait scp session failed: %w", err)
		}
		if err := os.Chmod(destinationPath, mode); err != nil {
			return fmt.Errorf("set local file mode failed: %w", err)
		}
		return nil
	}

	return withCopyRetry(copyOnce)
}

func withCopyRetry(copyOnce func() error) error {
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

func dialTargetClient(cfg BastionCopyConfig) (*ssh.Client, func(), error) {
	signer, err := loadPrivateKeySigner(cfg.BastionKey)
	if err != nil {
		return nil, nil, fmt.Errorf("load private key failed: %w", err)
	}

	bastionCfg := &ssh.ClientConfig{
		User:            cfg.BastionUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}

	bastionClient, err := ssh.Dial("tcp", cfg.BastionAddr, bastionCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh dial bastion failed: %w", err)
	}

	if cfg.DirectTarget {
		return bastionClient, func() { _ = bastionClient.Close() }, nil
	}

	targetClient, err := dialHostThroughBastion(
		bastionClient,
		cfg.TargetAddr,
		cfg.TargetUser,
		cfg.TargetPass,
		cfg.Timeout,
	)
	if err != nil {
		_ = bastionClient.Close()
		return nil, nil, err
	}

	cleanup := func() {
		_ = targetClient.Close()
		_ = bastionClient.Close()
	}
	return targetClient, cleanup, nil
}

func scpReadFileHeader(reader *bufio.Reader) (os.FileMode, int64, string, error) {
	headerType, err := reader.ReadByte()
	if err != nil {
		return 0, 0, "", fmt.Errorf("read scp header failed: %w", err)
	}

	switch headerType {
	case 'C':
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0, 0, "", fmt.Errorf("read scp header line failed: %w", err)
		}
		parts := strings.SplitN(strings.TrimSpace(line), " ", 3)
		if len(parts) != 3 {
			return 0, 0, "", fmt.Errorf("invalid scp file header: %q", strings.TrimSpace(line))
		}

		modeValue, err := strconv.ParseUint(parts[0], 8, 32)
		if err != nil {
			return 0, 0, "", fmt.Errorf("parse scp file mode failed: %w", err)
		}

		sizeValue, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || sizeValue < 0 {
			return 0, 0, "", fmt.Errorf("parse scp file size failed: %w", err)
		}

		name := filepath.Base(strings.TrimSpace(parts[2]))
		if name == "" || name == "." || name == "/" {
			return 0, 0, "", fmt.Errorf("invalid remote file name in scp header")
		}

		return os.FileMode(modeValue & 0o777), sizeValue, name, nil
	case 'D':
		return 0, 0, "", fmt.Errorf("copying directories is not supported")
	case 1, 2:
		line, _ := reader.ReadString('\n')
		return 0, 0, "", fmt.Errorf("scp error: %s", strings.TrimSpace(line))
	case 0:
		return 0, 0, "", fmt.Errorf("scp returned no file data")
	default:
		return 0, 0, "", fmt.Errorf("unexpected scp header byte: %d", headerType)
	}
}

func resolveLocalDestination(localPath, remoteName string) (string, error) {
	if strings.TrimSpace(localPath) == "" {
		return "", fmt.Errorf("local destination path is required")
	}

	info, err := os.Stat(localPath)
	if err == nil {
		if info.IsDir() {
			return filepath.Join(localPath, remoteName), nil
		}
		return localPath, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat local destination failed: %w", err)
	}

	if strings.HasSuffix(localPath, "/") || strings.HasSuffix(localPath, string(os.PathSeparator)) {
		if err := os.MkdirAll(localPath, 0o755); err != nil {
			return "", fmt.Errorf("create local destination directory failed: %w", err)
		}
		return filepath.Join(localPath, remoteName), nil
	}

	return localPath, nil
}

func scpShellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
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
