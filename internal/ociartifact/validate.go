// Package ociartifact verifies the bounded OCI image layout archive produced
// by BuildKit before Stealth publishes it as durable deployment output.
package ociartifact

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"slices"
	"strings"
)

const (
	maxIndexBytes  = 8 << 20
	maxLayoutBytes = 1024
	maxEntries     = 16384
)

var ErrInvalidArchive = errors.New("invalid OCI image archive")

type descriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

type index struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Manifests     []descriptor `json:"manifests"`
}

type imageManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	MediaType     string       `json:"mediaType"`
	Config        descriptor   `json:"config"`
	Layers        []descriptor `json:"layers"`
}

// ImageInfo contains the verified OCI identity and configuration needed by the
// trusted runtime importer.
type ImageInfo struct {
	ManifestDigest string
	ConfigDigest   string
	ArchiveSize    int64 // Exact size of the verified archive.
	OS             string
	Architecture   string
	Variant        string
	RuntimeConfig  RuntimeConfig
	LayerDigests   []string // OCI layer blob digests in manifest order.
	LayerDiffIDs   []string
	VolumePaths    []string
}

// RuntimeConfig contains the image configuration fields that Docker exposes
// for a created container. Docker's containerd image store may report the
// imported manifest digest as the image ID, so callers compare these fields
// along with the platform and root filesystem instead of relying on that ID.
type RuntimeConfig struct {
	Entrypoint   []string                  `json:"Entrypoint"`
	Command      []string                  `json:"Cmd"`
	Environment  []string                  `json:"Env"`
	WorkingDir   string                    `json:"WorkingDir"`
	User         string                    `json:"User"`
	Volumes      map[string]struct{}       `json:"Volumes"`
	ExposedPorts map[string]struct{}       `json:"ExposedPorts"`
	Labels       map[string]string         `json:"Labels"`
	StopSignal   string                    `json:"StopSignal"`
	Healthcheck  *RuntimeHealthcheckConfig `json:"Healthcheck"`
	OnBuild      []string                  `json:"OnBuild"`
	Shell        []string                  `json:"Shell"`
}

type RuntimeHealthcheckConfig struct {
	Test          []string `json:"Test"`
	Interval      int64    `json:"Interval"`
	Timeout       int64    `json:"Timeout"`
	StartPeriod   int64    `json:"StartPeriod"`
	StartInterval int64    `json:"StartInterval"`
	Retries       int      `json:"Retries"`
}

func (config RuntimeConfig) Equal(other RuntimeConfig) bool {
	return slices.Equal(config.Entrypoint, other.Entrypoint) &&
		slices.Equal(config.Command, other.Command) &&
		slices.Equal(config.Environment, other.Environment) &&
		config.WorkingDir == other.WorkingDir && config.User == other.User &&
		maps.Equal(config.Volumes, other.Volumes) && maps.Equal(config.ExposedPorts, other.ExposedPorts) &&
		maps.Equal(config.Labels, other.Labels) && config.StopSignal == other.StopSignal &&
		equalRuntimeHealthcheck(config.Healthcheck, other.Healthcheck) &&
		slices.Equal(config.OnBuild, other.OnBuild) && slices.Equal(config.Shell, other.Shell)
}

func equalRuntimeHealthcheck(left, right *RuntimeHealthcheckConfig) bool {
	if left == nil || right == nil {
		return left == right
	}
	return slices.Equal(left.Test, right.Test) && left.Interval == right.Interval && left.Timeout == right.Timeout &&
		left.StartPeriod == right.StartPeriod && left.StartInterval == right.StartInterval && left.Retries == right.Retries
}

type imageConfig struct {
	OS           string        `json:"os"`
	Architecture string        `json:"architecture"`
	Variant      string        `json:"variant"`
	Config       RuntimeConfig `json:"config"`
	RootFS       struct {
		Type    string   `json:"type"`
		DiffIDs []string `json:"diff_ids"`
	} `json:"rootfs"`
}

// Validate verifies safe tar paths, an OCI layout marker, the index and every
// sha256 blob. The requested digest must identify an image manifest referenced
// by index.json; archive checksum identity remains a separate concern.
func Validate(reader io.Reader, expectedDigest string, maxBytes int64) error {
	if !validDigest(expectedDigest) || maxBytes <= 0 {
		return ErrInvalidArchive
	}
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	archive := tar.NewReader(limited)
	seen := make(map[string]struct{})
	blobs := make(map[string]int64)
	var total int64
	var fileCount int
	var layoutSeen, indexSeen bool
	var imageIndex index
	var expectedManifest []byte
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: read tar entry", ErrInvalidArchive)
		}
		name, err := safeName(header.Name)
		if err != nil {
			return err
		}
		if _, duplicate := seen[name]; duplicate {
			return fmt.Errorf("%w: duplicate archive path", ErrInvalidArchive)
		}
		seen[name] = struct{}{}
		if header.Typeflag == tar.TypeDir {
			if name != "." && name != "blobs" && name != "blobs/sha256" {
				return fmt.Errorf("%w: unexpected directory", ErrInvalidArchive)
			}
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 {
			return fmt.Errorf("%w: links and special files are not allowed", ErrInvalidArchive)
		}
		fileCount++
		if fileCount > maxEntries || header.Size > maxBytes-total {
			return fmt.Errorf("%w: archive exceeds configured bounds", ErrInvalidArchive)
		}
		total += header.Size
		switch name {
		case "oci-layout":
			if header.Size > maxLayoutBytes {
				return ErrInvalidArchive
			}
			data, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
			if err != nil || int64(len(data)) != header.Size {
				return ErrInvalidArchive
			}
			var marker struct {
				Version string `json:"imageLayoutVersion"`
			}
			if json.Unmarshal(data, &marker) != nil || marker.Version != "1.0.0" {
				return ErrInvalidArchive
			}
			layoutSeen = true
		case "index.json":
			if header.Size > maxIndexBytes {
				return ErrInvalidArchive
			}
			data, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
			if err != nil || int64(len(data)) != header.Size || json.Unmarshal(data, &imageIndex) != nil ||
				imageIndex.SchemaVersion != 2 ||
				len(imageIndex.Manifests) == 0 {
				return ErrInvalidArchive
			}
			indexSeen = true
		default:
			if !strings.HasPrefix(name, "blobs/sha256/") {
				return fmt.Errorf("%w: unexpected OCI layout path", ErrInvalidArchive)
			}
			digest := strings.TrimPrefix(name, "blobs/sha256/")
			if !validHexDigest(digest) {
				return ErrInvalidArchive
			}
			hasher := sha256.New()
			var manifest bytes.Buffer
			var destination io.Writer = hasher
			if digest == strings.TrimPrefix(expectedDigest, "sha256:") {
				if header.Size <= 0 || header.Size > maxIndexBytes {
					return ErrInvalidArchive
				}
				destination = io.MultiWriter(hasher, &manifest)
			}
			written, err := io.CopyN(destination, archive, header.Size)
			if err != nil || written != header.Size || hex.EncodeToString(hasher.Sum(nil)) != digest {
				return fmt.Errorf("%w: blob checksum mismatch", ErrInvalidArchive)
			}
			blobs[digest] = header.Size
			if manifest.Len() > 0 {
				expectedManifest = manifest.Bytes()
			}
		}
	}
	_, copyErr := io.Copy(io.Discard, limited)
	copyIncomplete := copyErr != nil || limited.N == 0
	layoutIncomplete := !layoutSeen || !indexSeen || total > maxBytes
	if copyIncomplete || layoutIncomplete {
		return fmt.Errorf("%w: OCI layout is incomplete", ErrInvalidArchive)
	}
	archiveSize := maxBytes + 1 - limited.N
	if archiveSize > maxBytes {
		return fmt.Errorf("%w: archive exceeds configured bounds", ErrInvalidArchive)
	}
	foundExpected := false
	var expectedMediaType string
	for _, item := range imageIndex.Manifests {
		if !validDigest(item.Digest) || item.Size <= 0 ||
			blobs[strings.TrimPrefix(item.Digest, "sha256:")] != item.Size {
			return fmt.Errorf("%w: index descriptor is missing its blob", ErrInvalidArchive)
		}
		if item.Digest == expectedDigest {
			if item.MediaType != "application/vnd.oci.image.manifest.v1+json" &&
				item.MediaType != "application/vnd.docker.distribution.manifest.v2+json" {
				return fmt.Errorf("%w: selected descriptor is not an image manifest", ErrInvalidArchive)
			}
			foundExpected = true
			expectedMediaType = item.MediaType
		}
	}
	if !foundExpected {
		return fmt.Errorf("%w: BuildKit digest is absent from OCI index", ErrInvalidArchive)
	}
	var image imageManifest
	if len(expectedManifest) == 0 || json.Unmarshal(expectedManifest, &image) != nil || image.SchemaVersion != 2 ||
		image.MediaType != expectedMediaType {
		return fmt.Errorf("%w: image manifest is invalid", ErrInvalidArchive)
	}
	configSize, configExists := blobs[strings.TrimPrefix(image.Config.Digest, "sha256:")]
	if !validDigest(image.Config.Digest) || image.Config.Size <= 0 || !configExists || configSize != image.Config.Size {
		return fmt.Errorf("%w: image config descriptor is missing its blob", ErrInvalidArchive)
	}
	for _, layer := range image.Layers {
		layerSize, layerExists := blobs[strings.TrimPrefix(layer.Digest, "sha256:")]
		if !validDigest(layer.Digest) || layer.Size <= 0 || !layerExists || layerSize != layer.Size {
			return fmt.Errorf("%w: layer descriptor is missing its blob", ErrInvalidArchive)
		}
	}
	return nil
}

// Inspect revalidates a seekable persisted archive and extracts the selected
// manifest's config identity and bounded runtime metadata. The external
// AppDeployment digest remains the OCI manifest digest and must never be
// replaced with a local Docker image ID.
func Inspect(reader io.ReadSeeker, expectedDigest string, maxBytes int64) (ImageInfo, error) {
	if reader == nil || !validDigest(expectedDigest) || maxBytes <= 0 {
		return ImageInfo{}, ErrInvalidArchive
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return ImageInfo{}, ErrInvalidArchive
	}
	if err := Validate(reader, expectedDigest, maxBytes); err != nil {
		return ImageInfo{}, err
	}
	archiveSize, err := reader.Seek(0, io.SeekEnd)
	if err != nil || archiveSize <= 0 || archiveSize > maxBytes {
		return ImageInfo{}, ErrInvalidArchive
	}
	manifestBytes, err := readBlob(reader, expectedDigest, maxIndexBytes, maxBytes)
	if err != nil || archiveDigestBytes(manifestBytes) != expectedDigest {
		return ImageInfo{}, ErrInvalidArchive
	}
	var manifest imageManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || !validDigest(manifest.Config.Digest) ||
		manifest.Config.Size <= 0 ||
		manifest.Config.Size > maxIndexBytes {
		return ImageInfo{}, ErrInvalidArchive
	}
	configBytes, err := readBlob(reader, manifest.Config.Digest, maxIndexBytes, maxBytes)
	if err != nil || int64(len(configBytes)) != manifest.Config.Size ||
		archiveDigestBytes(configBytes) != manifest.Config.Digest {
		return ImageInfo{}, ErrInvalidArchive
	}
	var config imageConfig
	if json.Unmarshal(configBytes, &config) != nil || config.OS == "" || config.Architecture == "" ||
		config.RootFS.Type != "layers" ||
		len(config.RootFS.DiffIDs) != len(manifest.Layers) {
		return ImageInfo{}, ErrInvalidArchive
	}
	for _, digest := range config.RootFS.DiffIDs {
		if !validDigest(digest) {
			return ImageInfo{}, ErrInvalidArchive
		}
	}
	volumes := make([]string, 0, len(config.Config.Volumes))
	for name := range config.Config.Volumes {
		if name == "" || strings.ContainsAny(name, "\x00\r\n") {
			return ImageInfo{}, ErrInvalidArchive
		}
		volumes = append(volumes, name)
	}
	slices.Sort(volumes)
	return ImageInfo{
		ManifestDigest: expectedDigest,
		ConfigDigest:   manifest.Config.Digest,
		ArchiveSize:    archiveSize,
		OS:             config.OS,
		Architecture:   config.Architecture,
		Variant:        config.Variant,
		RuntimeConfig:  config.Config,
		LayerDigests:   layerDigests(manifest.Layers),
		LayerDiffIDs:   append([]string(nil), config.RootFS.DiffIDs...),
		VolumePaths:    volumes,
	}, nil
}

func layerDigests(layers []descriptor) []string {
	digests := make([]string, len(layers))
	for index, layer := range layers {
		digests[index] = layer.Digest
	}
	return digests
}

func readBlob(reader io.ReadSeeker, digest string, maxMetadataBytes, maxArchiveBytes int64) ([]byte, error) {
	if !validDigest(digest) || maxArchiveBytes <= 0 {
		return nil, ErrInvalidArchive
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return nil, ErrInvalidArchive
	}
	limited := &io.LimitedReader{R: reader, N: maxArchiveBytes + 1}
	archive := tar.NewReader(limited)
	wanted := "blobs/sha256/" + strings.TrimPrefix(digest, "sha256:")
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil, ErrInvalidArchive
		}
		if err != nil {
			return nil, ErrInvalidArchive
		}
		name, err := safeName(header.Name)
		if err != nil {
			return nil, err
		}
		if name != wanted {
			continue
		}
		if (header.Typeflag != tar.TypeReg) || header.Size <= 0 ||
			header.Size > maxMetadataBytes {
			return nil, ErrInvalidArchive
		}
		data, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
		if err != nil || int64(len(data)) != header.Size {
			return nil, ErrInvalidArchive
		}
		return data, nil
	}
}

func archiveDigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func safeName(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\\x00\r\n") {
		return "", ErrInvalidArchive
	}
	name := strings.TrimSuffix(value, "/")
	if name == "." {
		return ".", nil
	}
	if path.Clean(name) != name || strings.HasPrefix(name, "../") {
		return "", ErrInvalidArchive
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrInvalidArchive
		}
		for _, r := range part {
			if r < 0x20 || r == 0x7f {
				return "", ErrInvalidArchive
			}
		}
	}
	return name, nil
}

func validDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validHexDigest(strings.TrimPrefix(value, "sha256:"))
}

func validHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
