// internal/web/handlers/stubs.go
// Stub handler types — replaced by real implementations in subsequent tasks.
// This file exists only to allow server.go to compile before handlers are implemented.
package handlers

import "net/http"

// placeholder satisfies the import to avoid empty file errors.
var _ = http.StatusOK
