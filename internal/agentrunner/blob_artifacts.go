package agentrunner

import (
	"context"
	"io"

	"github.com/Nafixhutao/stealth-forge/internal/storage"
	"github.com/google/uuid"
)

// ArtifactStore saves a produced file and returns its opaque relative path.
type ArtifactStore interface {
	Save(ctx context.Context, artifactID, projectID uuid.UUID, name string, r io.Reader) (string, error)
}

type blobArtifactStore struct {
	store storage.BlobStore
	max   int64
}

func NewBlobArtifactStore(store storage.BlobStore, max int64) ArtifactStore {
	return &blobArtifactStore{store: store, max: max}
}

// Agent artifacts are not user-visible buckets: the bucket id only shapes the
// opaque blob path, and the artifact row owns the download.
var agentArtifactBucketID = uuid.MustParse("00000000-0000-7000-8000-000000000000")

func (s *blobArtifactStore) Save(
	ctx context.Context,
	artifactID,
	projectID uuid.UUID,
	name string,
	r io.Reader,
) (string, error) {
	prepared, err := s.store.BeginUploadWithLimit(
		ctx, projectID, agentArtifactBucketID, artifactID, r, name, s.max,
	)
	if err != nil {
		return "", err
	}
	defer s.store.Cleanup(&prepared)
	if err := s.store.Commit(ctx, &prepared); err != nil {
		return "", err
	}
	return prepared.RelativePath, nil
}
