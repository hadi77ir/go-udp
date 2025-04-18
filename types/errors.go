package types

import "errors"

var (
	ErrUnknownNetwork      = errors.New("udp: unknown network")
	ErrMissingAddr         = errors.New("udp: missing address")
	ErrAlreadyInUse        = errors.New("udp: already in use")
	ErrUnexpectedNil       = errors.New("udp: unexpected nil")
	ErrClosedSuperConn     = errors.New("udp: supConn closed")
	ErrListenQueueExceeded = errors.New("udp: listen queue exceeded")
)
