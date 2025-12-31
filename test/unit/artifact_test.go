package unit

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockfiler "github.com/w-h-a/gomento/internal/client/filer/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1artifact "github.com/w-h-a/gomento/internal/service/v1_artifact"
)

func TestService_Artifact_UploadFile(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// 1. Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()

	s := v1artifact.NewV1Service(p, f)

	artifactId := uuid.New()
	ctx := context.Background()

	require.NoError(t, p.CreateArtifact(ctx, &v1.Artifact{Id: artifactId, ProjectId: uuid.New()}))

	// 2. Act
	content := "package main"
	file, err := s.UploadFile(ctx, v1artifact.CreateFileInput{
		ArtifactId: artifactId,
		Path:       "/src",
		Filename:   "main.go",
		MimeType:   "text/x-go",
		Size:       int64(len(content)),
		Reader:     strings.NewReader(content),
	})
	require.NoError(t, err)

	// 3. Assert (Observable Result)
	assert.NotNil(t, file)
	assert.Equal(t, "main.go", file.Filename)
	assert.Equal(t, "/src", file.Path)
	assert.NotEqual(t, uuid.Nil, file.AssetId)

	// 4. Assert (Observable State Persistence)
	savedFiles, err := p.ListFiles(ctx, artifactId)
	assert.NoError(t, err)
	assert.Len(t, savedFiles, 1)

	saved := savedFiles[0]
	assert.Equal(t, file.Id, saved.Id)
	assert.Equal(t, file.AssetId, saved.Asset.Id)
	assert.Equal(t, "uploads/main.go", saved.Asset.Path)

	// Act
	updatedContent := "package main\n\nfunc main() {}"
	updatedFile, err := s.UploadFile(ctx, v1artifact.CreateFileInput{
		ArtifactId: artifactId,
		Path:       "/src",
		Filename:   "main.go",
		MimeType:   "text/x-go",
		Size:       int64(len(updatedContent)),
		Reader:     strings.NewReader(updatedContent),
	})
	require.NoError(t, err)

	// Assert (Upsert Behavior)
	assert.Equal(t, file.Id, updatedFile.Id, "File ID should be the same on upsert")
	assert.NotEqual(t, file.AssetId, updatedFile.AssetId, "Asset ID should be different for new content")

	savedFiles, err = p.ListFiles(ctx, artifactId)
	assert.NoError(t, err)
	assert.Len(t, savedFiles, 1, "Should still only be one file after upsert")
}

func TestService_GetFile(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()

	s := v1artifact.NewV1Service(p, f)

	ctx := context.Background()

	artifactId := uuid.New()
	_ = p.CreateArtifact(ctx, &v1.Artifact{Id: artifactId, ProjectId: uuid.New()})

	expectedPath := "src/main.go"
	assetPath := "uploads/123/main.go"

	fileId := uuid.New()
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id:         fileId,
		ArtifactId: artifactId,
		Path:       "src",
		Filename:   "main.go",
	}, &v1.Asset{
		Id:   uuid.New(),
		Path: assetPath,
	})

	// Act
	file, url, err := s.GetFile(ctx, artifactId, expectedPath, false)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, fileId, file.Id)
	assert.Empty(t, url, "URL should be empty when not requested")

	// Act
	file, url, err = s.GetFile(ctx, artifactId, expectedPath, true)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, fileId, file.Id)
	assert.Contains(t, url, "https://mock", "Should return a presigned URL")
	assert.Contains(t, url, assetPath, "URL should point to the physical asset")
}
