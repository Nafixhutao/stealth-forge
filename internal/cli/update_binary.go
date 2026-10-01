package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func extractUpdateBinary(archive []byte) (string, string, error) {
	temporaryDir, err := os.MkdirTemp("", "stealth-update-")
	if err != nil {
		return "", "", fmt.Errorf("create secure temporary directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(temporaryDir)
	}

	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		cleanup()
		return "", "", fmt.Errorf("read gzip archive: %w", err)
	}
	tarReader := tar.NewReader(reader)
	found := false
	temporaryBinary := filepath.Join(temporaryDir, "stealth")
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("read tar archive: %w", nextErr)
		}
		if header.Name != "stealth" || path.IsAbs(header.Name) || filepath.IsAbs(header.Name) || path.Clean(header.Name) != header.Name {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("release archive contains unexpected path %q", header.Name)
		}
		if found {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("release archive contains multiple Stealth binaries")
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("release archive entry %q is not a regular file", header.Name)
		}
		if header.Size <= 0 || header.Size > maxUpdateBinarySize {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("release binary has an invalid size")
		}
		file, openErr := os.OpenFile(temporaryBinary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if openErr != nil {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("create extracted binary: %w", openErr)
		}
		written, copyErr := io.CopyN(file, tarReader, header.Size)
		syncErr := file.Sync()
		closeErr := file.Close()
		if copyErr != nil || written != header.Size || syncErr != nil || closeErr != nil {
			cleanup()
			if copyErr != nil {
				return "", "", fmt.Errorf("extract Stealth binary: %w", copyErr)
			}
			return "", "", fmt.Errorf("extract Stealth binary: incomplete or unsafely persisted file")
		}
		if err := os.Chmod(temporaryBinary, 0755); err != nil {
			_ = reader.Close()
			cleanup()
			return "", "", fmt.Errorf("make extracted binary executable: %w", err)
		}
		found = true
	}
	if err := reader.Close(); err != nil {
		cleanup()
		return "", "", fmt.Errorf("close release archive: %w", err)
	}
	if !found {
		cleanup()
		return "", "", fmt.Errorf("release archive does not contain the Stealth binary")
	}
	return temporaryDir, temporaryBinary, nil
}

func (a *App) validateDownloadedBinary(ctx context.Context, binaryPath, expectedVersion string) error {
	info, err := os.Lstat(binaryPath)
	if err != nil {
		return fmt.Errorf("cannot inspect downloaded Stealth binary: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Mode()&0111 == 0 {
		return fmt.Errorf("downloaded Stealth binary is not a non-empty executable file")
	}
	runner := a.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	output, err := runner.Output(ctx, "", binaryPath, "version")
	if err != nil {
		return fmt.Errorf("downloaded Stealth binary failed version validation: %w", err)
	}
	firstLine := strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
	if firstLine != "Stealth "+expectedVersion {
		return fmt.Errorf("downloaded Stealth binary reported %q, expected Stealth %s", firstLine, expectedVersion)
	}
	return nil
}

func (a *App) replaceExecutable(ctx context.Context, source, target string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update canceled: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".stealth-update-")
	if err != nil {
		return fmt.Errorf("cannot update this installation because %s is not writable; re-run the update with appropriate system permissions: %w", target, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	sourceFile, err := os.Open(source)
	if err != nil {
		_ = temporary.Close()
		return fmt.Errorf("open validated Stealth binary: %w", err)
	}
	_, copyErr := io.Copy(temporary, sourceFile)
	closeSourceErr := sourceFile.Close()
	if copyErr != nil {
		_ = temporary.Close()
		return fmt.Errorf("stage Stealth binary: %w", copyErr)
	}
	if closeSourceErr != nil {
		_ = temporary.Close()
		return fmt.Errorf("close validated Stealth binary: %w", closeSourceErr)
	}
	if err := temporary.Chmod(0755); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set staged Stealth binary permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("persist staged Stealth binary: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close staged Stealth binary: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update canceled: %w", err)
	}
	rename := a.renameFile
	if rename == nil {
		rename = os.Rename
	}
	if err := rename(temporaryPath, target); err != nil {
		return fmt.Errorf("could not replace %s; the current installation was left unchanged: %w", target, err)
	}
	return nil
}

func (a *App) printUpdateHeader() {
	if a.hasInteractiveTerminal() {
		fmt.Fprintln(a.out, titleStyle.Render("Stealth Update"))
		return
	}
	fmt.Fprintln(a.out, "Stealth Update")
}

func (a *App) printUpdateStep(mark, message string, style lipgloss.Style) {
	line := mark + " " + message
	if a.hasInteractiveTerminal() {
		fmt.Fprintln(a.out, style.Render(line))
		return
	}
	fmt.Fprintln(a.out, line)
}

func (a *App) printUpdateSuccess(message string) {
	line := "✓ " + message
	if a.hasInteractiveTerminal() {
		fmt.Fprintln(a.out, successStyle.Render(line))
		return
	}
	fmt.Fprintln(a.out, line)
}

func (a *App) printUpdateFailure(err error) {
	line := "✗ Update failed"
	if a.hasInteractiveTerminal() {
		fmt.Fprintln(a.errOut, errorStyle.Render(line))
	} else {
		fmt.Fprintln(a.errOut, line)
	}
	fmt.Fprintln(a.errOut, err)
	var migrated *platformMigratedUpdateError
	if errors.As(err, &migrated) {
		fmt.Fprintf(a.errOut, "The installed CLI was not replaced, but the platform migration reached %s. Re-run `stealth update` to reconcile the CLI.\n", migrated.targetVersion)
		return
	}
	fmt.Fprintln(a.errOut, "Your existing Stealth CLI was not modified.")
}
