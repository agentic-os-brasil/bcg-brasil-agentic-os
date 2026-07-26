//go:build !darwin && !windows

package longrun

// DefaultAnchor remains unavailable until a host-specific credential-service
// adapter supplies an anchor with the same monotonic contract.
func DefaultAnchor() (MonotonicAnchor, error) { return nil, ErrMonotonicAnchorUnavailable }
