package sdkbackend

import "pkg/ark"

// Compile-time interface satisfaction checks.
var _ ark.ContainerEngine = (*SDKEngine)(nil)
