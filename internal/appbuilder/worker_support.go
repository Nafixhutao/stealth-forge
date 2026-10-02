package appbuilder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func safeWorkerID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func safeRelativeArtifact(value string) bool {
	parts := strings.Split(value, "/")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		id, err := uuid.Parse(part)
		if err != nil || id == uuid.Nil || id.Version() != uuid.Version(7) || id.String() != part {
			return false
		}
	}
	return true
}

func insideRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func valueOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func checksumSeekable(file io.ReadSeeker) (string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type progressWriter struct {
	ctx          context.Context
	store        Persistence
	workerID     string
	projectID    uuid.UUID
	appID        uuid.UUID
	deploymentID uuid.UUID
	redactRoot   string
	buffer       []byte
	retained     int
	lines        int
	truncated    bool
}

func (w *progressWriter) Write(data []byte) (int, error) {
	written := len(data)
	for _, char := range data {
		if char == '\n' {
			w.writeLine(w.buffer)
			w.buffer = w.buffer[:0]
			continue
		}
		if len(w.buffer) < 16<<10 {
			w.buffer = append(w.buffer, char)
		} else {
			w.truncated = true
		}
	}
	return written, nil
}

func (w *progressWriter) Flush() {
	if len(w.buffer) > 0 {
		w.writeLine(w.buffer)
		w.buffer = nil
	}
	if w.truncated {
		w.writeLine([]byte("Build output truncated after reaching the retention limit"))
	}
}

func (w *progressWriter) writeLine(raw []byte) {
	if w.lines >= maxRetainedLogLines || w.retained >= maxRetainedLogBytes {
		w.truncated = true
		return
	}
	line := strings.ToValidUTF8(string(raw), "�")
	line = strings.ReplaceAll(line, w.redactRoot, "<worker-staging>")
	line = strings.ReplaceAll(line, "/run/secrets/stealth-buildkit", "<BuildKit TLS credential directory>")
	line = strings.ReplaceAll(line, "\r", " ")
	line = strings.ReplaceAll(line, "\x00", " ")
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if len(line) > 4096 {
		line = line[:4096]
	}
	for len(line) > 0 && len(line)+w.retained > maxRetainedLogBytes {
		line = line[:len(line)-1]
	}
	if line == "" {
		w.truncated = true
		return
	}
	id, err := uuid.NewV7()
	if err == nil {
		_, _ = w.store.AppendAppBuildLog(w.ctx, w.projectID, w.appID, w.deploymentID, w.workerID, id, "info", line)
	}
	w.lines++
	w.retained += len(line)
}
