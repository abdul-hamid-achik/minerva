package cli

import (
	"errors"
	"fmt"
)

// Exit codes: 0 ok, 1 command or lint error.
type ExitCode int

func (e ExitCode) Error() string {
	return fmt.Sprintf("exit status %d", int(e))
}

// Code returns the numeric exit status.
func (e ExitCode) Code() int { return int(e) }

// AsExitCode reports whether err is (or wraps) an ExitCode.
func AsExitCode(err error) (int, bool) {
	var code ExitCode
	if errors.As(err, &code) {
		return code.Code(), true
	}
	return 0, false
}
