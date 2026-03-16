package bastion

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type commandHandler func(cmd string) (stdout string, stderr string, exitCode int)

func generateSigner(t *testing.T) (*rsa.PrivateKey, ssh.Signer) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	return key, signer
}

func writePrivateKey(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()

	der := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}

	dir := t.TempDir()
	path := filepath.Join(dir, "id_rsa")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pemBlock), 0o600); err != nil {
		t.Fatalf("write private key: %v", err)
	}

	return path
}

func startTargetSSHServer(t *testing.T, user, pass string, handler commandHandler) (string, func()) {
	t.Helper()

	_, hostSigner := generateSigner(t)
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if conn.User() == user && string(p) == pass {
				return nil, nil
			}
			return nil, fmt.Errorf("unauthorized")
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen target ssh: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			go serveSSHConnection(conn, cfg, handler, false)
		}
	}()

	stop := func() {
		close(done)
		_ = ln.Close()
	}

	return ln.Addr().String(), stop
}

func startBastionSSHServer(t *testing.T, clientPub ssh.PublicKey, handler commandHandler) (string, func()) {
	t.Helper()

	_, hostSigner := generateSigner(t)
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), clientPub.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("unauthorized key")
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen bastion ssh: %v", err)
	}

	done := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			go serveSSHConnection(conn, cfg, handler, true)
		}
	}()

	stop := func() {
		close(done)
		_ = ln.Close()
	}

	return ln.Addr().String(), stop
}

func serveSSHConnection(conn net.Conn, cfg *ssh.ServerConfig, handler commandHandler, allowDirect bool) {
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		switch newChannel.ChannelType() {
		case "session":
			ch, reqs, err := newChannel.Accept()
			if err != nil {
				continue
			}
			go handleSessionChannel(ch, reqs, handler)
		case "direct-tcpip":
			if !allowDirect {
				_ = newChannel.Reject(ssh.UnknownChannelType, "direct-tcpip not allowed")
				continue
			}
			var req directTCPIPReq
			if err := ssh.Unmarshal(newChannel.ExtraData(), &req); err != nil {
				_ = newChannel.Reject(ssh.Prohibited, "bad payload")
				continue
			}
			targetAddr := net.JoinHostPort(req.Host, strconv.Itoa(int(req.Port)))
			targetConn, err := net.Dial("tcp", targetAddr)
			if err != nil {
				_ = newChannel.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}
			ch, reqs, err := newChannel.Accept()
			if err != nil {
				_ = targetConn.Close()
				continue
			}
			go ssh.DiscardRequests(reqs)

			go func() {
				defer ch.Close()
				defer targetConn.Close()
				_, _ = io.Copy(targetConn, ch)
			}()

			go func() {
				defer ch.Close()
				defer targetConn.Close()
				_, _ = io.Copy(ch, targetConn)
			}()
		default:
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
		}
	}
}

func handleSessionChannel(ch ssh.Channel, reqs <-chan *ssh.Request, handler commandHandler) {
	defer ch.Close()

	for req := range reqs {
		if req.Type != "exec" {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			continue
		}

		var payload struct {
			Command string
		}
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
			return
		}

		if req.WantReply {
			_ = req.Reply(true, nil)
		}

		stdout, stderr, exitCode := handler(payload.Command)
		if stdout != "" {
			_, _ = ch.Write([]byte(stdout))
		}
		if stderr != "" {
			_, _ = ch.Stderr().Write([]byte(stderr))
		}

		status := struct {
			Status uint32
		}{Status: uint32(exitCode)}
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(&status))
		return
	}
}

type directTCPIPReq struct {
	Host           string
	Port           uint32
	OriginatorIP   string
	OriginatorPort uint32
}

func TestExecCommandOnBastionSuccess(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "hello from bastion", "", 0
	})
	defer stopBastion()

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		Command:     "anything",
	}

	res, err := ExecCommandOnBastion(cfg)
	if err != nil {
		t.Fatalf("ExecCommandOnBastion error: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hello from bastion" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestExecCommandOnBastionNonZeroExit(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "boom", 7
	})
	defer stopBastion()

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		Command:     "fails",
	}

	res, err := ExecCommandOnBastion(cfg)
	if err != nil {
		t.Fatalf("ExecCommandOnBastion error: %v", err)
	}

	if res.ExitCode != 7 {
		t.Fatalf("exit code mismatch: %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stderr) != "boom" {
		t.Fatalf("stderr mismatch: %q", res.Stderr)
	}
}

func TestExecCommandViaBastionSuccess(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	const targetUser = "target"
	const targetPass = "secret"

	targetAddr, stopTarget := startTargetSSHServer(t, targetUser, targetPass, func(cmd string) (string, string, int) {
		return "from target: " + cmd, "", 0
	})
	defer stopTarget()

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopBastion()

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		TargetUser:  targetUser,
		TargetAddr:  targetAddr,
		TargetPass:  targetPass,
		Command:     "echo hi",
	}

	res, err := ExecCommandViaBastion(cfg)
	if err != nil {
		t.Fatalf("ExecCommandViaBastion error: %v", err)
	}

	if res.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "from target: echo hi" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestInteractiveSessionOnBastionCallsSharedHelper(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopBastion()

	orig := startInteractiveSessionFn
	t.Cleanup(func() {
		startInteractiveSessionFn = orig
	})

	called := false
	startInteractiveSessionFn = func(client *ssh.Client) error {
		if client == nil {
			t.Fatalf("expected non-nil ssh client")
		}
		called = true
		return nil
	}

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
	}

	if err := InteractiveSessionOnBastion(cfg); err != nil {
		t.Fatalf("InteractiveSessionOnBastion error: %v", err)
	}
	if !called {
		t.Fatalf("expected shared interactive helper to be called")
	}
}

func TestInteractiveSessionViaBastionCallsSharedHelper(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	const targetUser = "target"
	const targetPass = "secret"

	targetAddr, stopTarget := startTargetSSHServer(t, targetUser, targetPass, func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopTarget()

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopBastion()

	orig := startInteractiveSessionFn
	t.Cleanup(func() {
		startInteractiveSessionFn = orig
	})

	called := false
	startInteractiveSessionFn = func(client *ssh.Client) error {
		if client == nil {
			t.Fatalf("expected non-nil ssh client")
		}
		called = true
		return nil
	}

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		TargetUser:  targetUser,
		TargetAddr:  targetAddr,
		TargetPass:  targetPass,
	}

	if err := InteractiveSessionViaBastion(cfg); err != nil {
		t.Fatalf("InteractiveSessionViaBastion error: %v", err)
	}
	if !called {
		t.Fatalf("expected shared interactive helper to be called")
	}
}

func TestInteractiveCommandOnBastionCallsSharedHelper(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopBastion()

	orig := startInteractiveCommandFn
	t.Cleanup(func() {
		startInteractiveCommandFn = orig
	})

	called := false
	startInteractiveCommandFn = func(client *ssh.Client, command string) error {
		if client == nil {
			t.Fatalf("expected non-nil ssh client")
		}
		if command != "bash" {
			t.Fatalf("unexpected command: %q", command)
		}
		called = true
		return nil
	}

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		Command:     "bash",
	}

	if err := InteractiveCommandOnBastion(cfg); err != nil {
		t.Fatalf("InteractiveCommandOnBastion error: %v", err)
	}
	if !called {
		t.Fatalf("expected shared interactive command helper to be called")
	}
}

func TestInteractiveCommandViaBastionCallsSharedHelper(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	const targetUser = "target"
	const targetPass = "secret"

	targetAddr, stopTarget := startTargetSSHServer(t, targetUser, targetPass, func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopTarget()

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		return "", "", 0
	})
	defer stopBastion()

	orig := startInteractiveCommandFn
	t.Cleanup(func() {
		startInteractiveCommandFn = orig
	})

	called := false
	startInteractiveCommandFn = func(client *ssh.Client, command string) error {
		if client == nil {
			t.Fatalf("expected non-nil ssh client")
		}
		if command != "bash -l" {
			t.Fatalf("unexpected command: %q", command)
		}
		called = true
		return nil
	}

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		TargetUser:  targetUser,
		TargetAddr:  targetAddr,
		TargetPass:  targetPass,
		Command:     "bash -l",
	}

	if err := InteractiveCommandViaBastion(cfg); err != nil {
		t.Fatalf("InteractiveCommandViaBastion error: %v", err)
	}
	if !called {
		t.Fatalf("expected shared interactive command helper to be called")
	}
}

func TestWaitPingViaBastionTimeout(t *testing.T) {
	clientKey, clientSigner := generateSigner(t)
	keyPath := writePrivateKey(t, clientKey)

	bastionAddr, stopBastion := startBastionSSHServer(t, clientSigner.PublicKey(), func(cmd string) (string, string, int) {
		if strings.Contains(cmd, "ping") {
			return "", "unreachable", 1
		}
		return "", "", 0
	})
	defer stopBastion()

	cfg := BastionExecConfig{
		BastionUser: "bastion",
		BastionAddr: bastionAddr,
		BastionKey:  keyPath,
		TargetAddr:  "10.0.0.1:22",
	}

	err := WaitPingViaBastion(context.Background(), cfg, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("unexpected error: %v", err)
	}
}
