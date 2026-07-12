package domain

import "errors"

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrNotFound            = errors.New("not found")
	ErrForbidden           = errors.New("forbidden")
	ErrInsufficientFunds   = errors.New("insufficient funds")
	ErrCurrencyMismatch    = errors.New("currency mismatch")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrTransferLimit       = errors.New("transfer limit exceeded")
	ErrRequestTooLarge     = errors.New("request body too large")
	ErrUnsupportedMedia    = errors.New("unsupported media type")
)
