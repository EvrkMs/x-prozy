package node

import "errors"

var (
	ErrNodeNotFound     = errors.New("node not found")
	ErrNodeNotConnected = errors.New("node not connected")
	ErrInvalidToken     = errors.New("invalid cluster token")
)
