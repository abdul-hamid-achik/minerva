package cli

import (
	"errors"
	"fmt"
)

// Exit code conventions for stack gates (CI / harness preflight):
//
//	0 — success (core healthy; retrieval ready when required)
//	1 — core stack incomplete / unhealthy
//	2 — core healthy but degraded (optional tools missing); only with --strict
//	3 — retrieval not ready (only with stack deep --require-retrieval)
//
// Other command failures still exit 1 via the default error path.

// ExitCode is a process exit status returned after successful output.
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
