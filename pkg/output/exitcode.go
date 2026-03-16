package output

import "errors"

// ExitCodeError carries a process exit code for command execution.
type ExitCodeError struct {
	Code   int
	Silent bool
}

// Error implements the error interface.
func (e *ExitCodeError) Error() string {
	return "command failed"
}

// NewSilentExitCodeError creates an exit-code error that should not print an extra message.
func NewSilentExitCodeError(code int) error {
	if code == 0 {
		return nil
	}
	return &ExitCodeError{
		Code:   code,
		Silent: true,
	}
}

// ExitCodeFromError extracts an exit code from an error, if present.
func ExitCodeFromError(err error) (int, bool) {
	var exitErr *ExitCodeError
	if errors.As(err, &exitErr) {
		return exitErr.Code, true
	}
	return 0, false
}
