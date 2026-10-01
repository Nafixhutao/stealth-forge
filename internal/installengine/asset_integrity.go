package installengine

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"strings"
)

// parseChecksumsManifest reads the release's sha256sum-style checksums.txt.
// It rejects malformed entries and duplicate paths so a later line cannot
// silently replace the digest selected for a managed asset.
func parseChecksumsManifest(contents []byte) (map[string]string, error) {
	if len(contents) == 0 || len(contents) > maxAssetSize {
		return nil, errors.New("checksum manifest is empty or too large")
	}
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	scanner.Buffer(make([]byte, 1024), maxAssetSize)
	var lineNumber int
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("checksum manifest line %d is malformed", lineNumber)
		}
		digest := strings.ToLower(fields[0])
		if _, err := hex.DecodeString(digest); err != nil {
			return nil, fmt.Errorf("checksum manifest line %d has an invalid SHA-256 digest", lineNumber)
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name == "" || strings.Contains(name, "\\") || path.IsAbs(name) || path.Clean(name) != name || name == "." ||
			strings.HasPrefix(name, "../") {
			return nil, fmt.Errorf("checksum manifest line %d has an invalid asset path", lineNumber)
		}
		if _, exists := checksums[name]; exists {
			return nil, fmt.Errorf("checksum manifest repeats asset %q", name)
		}
		checksums[name] = digest
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read checksum manifest: %w", err)
	}
	if len(checksums) == 0 {
		return nil, errors.New("checksum manifest has no entries")
	}
	return checksums, nil
}

func verifyManagedAssetChecksum(contents []byte, asset string, checksums map[string]string) error {
	expected, ok := checksums[asset]
	if !ok {
		return fmt.Errorf("release checksum manifest has no entry for managed asset %q", asset)
	}
	actual := sha256.Sum256(contents)
	if !strings.EqualFold(hex.EncodeToString(actual[:]), expected) {
		return fmt.Errorf("managed asset %q does not match the release SHA-256 checksum", asset)
	}
	return nil
}
