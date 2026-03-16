//go:build !windows

package bastion

import (
	"os"
	"syscall"

	"golang.org/x/crypto/ssh"
)

func resizeSignals() []os.Signal {
	return []os.Signal{syscall.SIGWINCH}
}

func forwardSignals() []os.Signal {
	return []os.Signal{
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
		syscall.SIGQUIT,
	}
}

func isResizeSignal(sig os.Signal) bool {
	return sig == syscall.SIGWINCH
}

func toSSHSignal(sig os.Signal) (ssh.Signal, bool) {
	switch sig {
	case os.Interrupt:
		return ssh.SIGINT, true
	case syscall.SIGTERM:
		return ssh.SIGTERM, true
	case syscall.SIGHUP:
		return ssh.SIGHUP, true
	case syscall.SIGQUIT:
		return ssh.SIGQUIT, true
	default:
		return "", false
	}
}
