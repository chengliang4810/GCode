package remote

import (
	"context"
	"errors"
	"net/http"

	"gcode/internal/contracts"
)

// Only directory queries use this status mapping; unrelated HTTP failures must
// not masquerade as filesystem worker saturation or the host's query deadline.
func writeDirectoryError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status = http.StatusGatewayTimeout
	case errors.Is(err, contracts.ErrDirectoryBrowseBusy):
		status = http.StatusServiceUnavailable
	}
	http.Error(w, err.Error(), status)
}

func directoryErrorCause(path string, status int) error {
	switch path {
	case "/v1/directories", "/v1/directory-locations", "/v1/directory-create":
	default:
		return nil
	}
	switch status {
	case http.StatusGatewayTimeout:
		return context.DeadlineExceeded
	case http.StatusServiceUnavailable:
		return contracts.ErrDirectoryBrowseBusy
	default:
		return nil
	}
}
