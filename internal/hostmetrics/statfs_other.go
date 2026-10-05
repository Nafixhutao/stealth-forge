//go:build !linux

package hostmetrics

import "errors"

// statfs is a placeholder for non-Linux hosts; the collector reports zero disk
// usage there instead of failing. The production target is Linux.
type statfs struct {
	Bsize  uint64
	Blocks uint64
	Bavail uint64
}

func statfsCall(string, *statfs) error {
	return errors.New("host disk metrics are unavailable on this platform")
}
