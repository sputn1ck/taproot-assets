package proofconfig

import "errors"

var (
	// ErrInvalidConfig is returned when configuration is invalid.
	ErrInvalidConfig = errors.New("invalid proof system configuration")
)
