//go:build windows

package bastion

import (
	"os"

	"golang.org/x/crypto/ssh"
)

func resizeSignals() []os.Signal {
	return nil
}

func forwardSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func isResizeSignal(sig os.Signal) bool {
	_ = sig
	return false
}

func toSSHSignal(sig os.Signal) (ssh.Signal, bool) {
	if sig == os.Interrupt {
		return ssh.SIGINT, true
	}
	return "", false
}
