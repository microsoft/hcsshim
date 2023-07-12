package cose

import "errors"

// Common errors
var (
	ErrAlgorithmMismatch     = errors.New("algorithm mismatch")
	ErrAlgorithmNotFound     = errors.New("algorithm not found")
	ErrAlgorithmNotSupported = errors.New("algorithm not supported")
	ErrEmptySignature        = errors.New("empty signature")
	ErrInvalidAlgorithm      = errors.New("invalid algorithm")
	ErrMissingPayload        = errors.New("missing payload")
	ErrNoSignatures          = errors.New("no signatures attached")
	ErrUnavailableHashFunc   = errors.New("hash function is not available")
	ErrVerification          = errors.New("verification error")
)
