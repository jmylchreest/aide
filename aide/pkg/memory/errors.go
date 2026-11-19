package memory

import "errors"

// ErrNotFound identifies an absent record across direct and remote stores.
var ErrNotFound = errors.New("not found")
