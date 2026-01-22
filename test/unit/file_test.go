package unit

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mockdispatcher "github.com/w-h-a/gomento/internal/client/dispatcher/v1_mock"
	mockembedder "github.com/w-h-a/gomento/internal/client/embedder/mock"
	v1mockfiler "github.com/w-h-a/gomento/internal/client/filer/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	"github.com/w-h-a/gomento/internal/service"
	v1file "github.com/w-h-a/gomento/internal/service/v1_file"
)

func TestUpload_Global(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// 1. Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// 2. Act
	content := "global config"
	file, err := s.Upload(ctx, v1file.CreateFileInput{
		SpaceId:  nil,
		Path:     "/etc",
		Filename: "config.yaml",
		MimeType: "text/yaml",
		Size:     int64(len(content)),
		Reader:   strings.NewReader(content),
	})
	require.NoError(t, err)

	// 3. Assert
	assert.NotNil(t, file)
	assert.Nil(t, file.SpaceId)
	assert.Equal(t, "config.yaml", file.Filename)
	assert.NotEqual(t, uuid.Nil, file.Id)

	savedFiles, err := p.ListFiles(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, savedFiles, 1)
	assert.Nil(t, savedFiles[0].SpaceId)
	assert.Equal(t, file.Id, savedFiles[0].Id)
}

func TestUpload_Space(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	spaceId := uuid.New()

	// Act
	content := "space config"
	file, err := s.Upload(ctx, v1file.CreateFileInput{
		SpaceId:  &spaceId,
		Path:     "/etc",
		Filename: "config.yaml",
		Reader:   strings.NewReader(content),
	})
	require.NoError(t, err)

	// Assert
	assert.NotNil(t, file)
	assert.Equal(t, spaceId, *file.SpaceId)

	savedFiles, err := p.ListFiles(ctx, &spaceId)
	require.NoError(t, err)
	assert.Len(t, savedFiles, 1)
	assert.Equal(t, spaceId, *savedFiles[0].SpaceId)
}

func TestUpload_Upserts(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// 1. First Upload
	content1 := "v1"
	file1, err := s.Upload(ctx, v1file.CreateFileInput{
		SpaceId:  nil,
		Path:     "/",
		Filename: "config.yaml",
		Reader:   strings.NewReader(content1),
		Size:     int64(len(content1)),
	})
	require.NoError(t, err)

	// 2. Second Upload (Same Path/Filename)
	content2 := "v2"
	file2, err := s.Upload(ctx, v1file.CreateFileInput{
		SpaceId:  nil,
		Path:     "/",
		Filename: "config.yaml",
		Reader:   strings.NewReader(content2),
		Size:     int64(len(content2)),
	})
	require.NoError(t, err)

	// Assert
	assert.Equal(t, file1.Id, file2.Id, "File ID should remain constant on upsert")
	assert.NotEqual(t, file1.AssetId, file2.AssetId, "Asset ID should rotate on upsert")

	// Verify persistence state
	savedFiles, err := p.ListFiles(ctx, nil)
	require.NoError(t, err)
	assert.Len(t, savedFiles, 1, "Should verify no duplicates exist")
	assert.Equal(t, file2.AssetId, savedFiles[0].AssetId, "Persistence should point to newest asset")
}

func TestUpload_DispatchesIngestJob(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	d := v1mockdispatcher.NewV1Dispatcher()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, d, f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// Act
	content := "package main\nfunc main() {}"
	file, err := s.Upload(ctx, v1file.CreateFileInput{
		Filename: "main.go",
		Reader:   strings.NewReader(content),
		Size:     int64(len(content)),
	})
	require.NoError(t, err)

	// Assert
	jobs := d.Jobs()
	assert.Len(t, jobs, 1)

	job := jobs[0]
	assert.Equal(t, v1.JobTypeIngestFile, job.Type)

	var payload v1.IngestFileJobPayload
	err = json.Unmarshal(job.Payload, &payload)
	assert.NoError(t, err)

	assert.Equal(t, file.Id, payload.FileId)
	assert.Nil(t, payload.SpaceId)
}

func TestDownload_RetrievesFullContent(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	content := "line1\nline2\nline3"
	uploadedFile, err := s.Upload(ctx, v1file.CreateFileInput{
		Filename: "test.txt",
		Reader:   strings.NewReader(content),
		Size:     int64(len(content)),
	})
	require.NoError(t, err)

	// Act
	rc, err := s.Download(ctx, v1file.ReadFileInput{
		FileId: uploadedFile.Id,
	})
	require.NoError(t, err)
	defer rc.Close()

	// Assert
	data, err := io.ReadAll(rc)
	assert.NoError(t, err)
	expected := "line1\nline2\nline3\n"
	assert.Equal(t, expected, string(data))
}

func TestDownload_FiltersByLines(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	content := "line1\nline2\nline3\nline4\nline5"
	uploadedFile, _ := s.Upload(ctx, v1file.CreateFileInput{
		Filename: "test.txt",
		Reader:   strings.NewReader(content),
		Size:     int64(len(content)),
	})

	// Act
	rc, err := s.Download(ctx, v1file.ReadFileInput{
		FileId:    uploadedFile.Id,
		StartLine: 2,
		EndLine:   4,
	})
	require.NoError(t, err)
	defer rc.Close()

	// Assert
	data, err := io.ReadAll(rc)
	assert.NoError(t, err)
	expected := "line2\nline3\nline4\n"
	assert.Equal(t, expected, string(data))
}

func TestDownload_FailsIfFileNotFound(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// Act
	rc, err := s.Download(ctx, v1file.ReadFileInput{
		FileId: uuid.New(),
	})

	// Assert
	assert.ErrorIs(t, err, service.ErrFileNotFound)
	assert.Nil(t, rc)
}

func TestDownload_PropagatesStreamErrors(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	fileId := uuid.New()
	p.UpsertFileWithAsset(ctx, &v1.File{
		Id: fileId, Filename: "ghost.txt",
	}, &v1.Asset{
		Id: uuid.New(), Path: "ghost/path",
	})

	// Act
	rc, err := s.Download(ctx, v1file.ReadFileInput{FileId: fileId})

	// Assert
	assert.Error(t, err)
	assert.Nil(t, rc)
	assert.Contains(t, err.Error(), "mock file not found")
}

func TestListFiles_FiltersByScope(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	spaceId := uuid.New()

	// Create Global File
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id: uuid.New(), SpaceId: nil, Path: "/", Filename: "global.txt",
	}, &v1.Asset{Id: uuid.New()})

	// Create Space File
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id: uuid.New(), SpaceId: &spaceId, Path: "/", Filename: "local.txt",
	}, &v1.Asset{Id: uuid.New()})

	// Act 1: List Global
	globalOut, err := s.List(ctx, v1file.ListFilesInput{SpaceId: nil})
	require.NoError(t, err)

	// Assert
	assert.Len(t, globalOut.Items, 1)
	assert.Equal(t, "global.txt", globalOut.Items[0].Filename)

	// Act 2: List Space
	spaceOut, err := s.List(ctx, v1file.ListFilesInput{SpaceId: &spaceId})
	require.NoError(t, err)

	// Assert
	assert.Len(t, spaceOut.Items, 1)
	assert.Equal(t, "local.txt", spaceOut.Items[0].Filename)
}

func TestListFiles_FiltersByPath(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	spaceId := uuid.New()

	// 1. Create file in root
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id: uuid.New(), SpaceId: &spaceId, Path: "/", Filename: "root_doc.txt",
	}, &v1.Asset{Id: uuid.New()})

	// 2. Create file in subdirectory
	_ = p.UpsertFileWithAsset(ctx, &v1.File{
		Id: uuid.New(), SpaceId: &spaceId, Path: "src/services", Filename: "main.go",
	}, &v1.Asset{Id: uuid.New()})

	// Act: Filter by path "src/"
	out, err := s.List(ctx, v1file.ListFilesInput{
		SpaceId:    &spaceId,
		PathPrefix: "src",
	})
	require.NoError(t, err)

	// Assert
	assert.Len(t, out.Items, 1)
	assert.Equal(t, "main.go", out.Items[0].Filename)
	assert.Equal(t, "src/services", out.Items[0].Path)
}

func TestGetFile_ById(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	fileId := uuid.New()
	assetId := uuid.New()

	// Setup: Upsert a file directly to persistence
	err := p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		SpaceId:  nil,
		Path:     "src",
		Filename: "main.go",
	}, &v1.Asset{
		Id:   assetId,
		Path: "uploads/main.go",
	})
	require.NoError(t, err)

	// Act: Get without URL
	file, url, err := s.Get(ctx, fileId, false)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, fileId, file.Id)
	assert.Equal(t, "main.go", file.Filename)
	assert.Empty(t, url, "URL should be empty when withUrl is false")

	// Act: Get with URL
	file, url, err = s.Get(ctx, fileId, true)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, fileId, file.Id)
	assert.NotEmpty(t, url)
	assert.Contains(t, url, "uploads/main.go")
}

func TestConnectToSpace_File(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	f := v1mockfiler.NewV1Filer()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), f, mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// 1. Create Global File
	fileId := uuid.New()
	p.UpsertFileWithAsset(ctx, &v1.File{
		Id: fileId, SpaceId: nil, Path: "/", Filename: "orphan.txt",
	}, &v1.Asset{Id: uuid.New()})

	// 2. Create Space
	spaceId := uuid.New()
	p.CreateSpace(ctx, &v1.Space{Id: spaceId, Name: "Target Space"})

	// Act
	err := s.ConnectToSpace(ctx, fileId, spaceId)
	require.NoError(t, err)

	// Assert
	file, err := p.GetFile(ctx, fileId)
	require.NoError(t, err)
	require.NotNil(t, file.SpaceId)
	assert.Equal(t, spaceId, *file.SpaceId)
}

func TestConnectToSpace_FailsIfFileMissing(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockfiler.NewV1Filer(), mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// Act
	err := s.ConnectToSpace(ctx, uuid.New(), uuid.New())

	// Assert
	assert.ErrorIs(t, err, service.ErrFileNotFound)
}

func TestConnectToSpace_FailsIfSpaceMissing(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1file.NewV1Service(p, v1mockdispatcher.NewV1Dispatcher(), v1mockfiler.NewV1Filer(), mockembedder.NewEmbedder(), "q")

	ctx := context.Background()

	// Create File
	fileId := uuid.New()
	p.UpsertFileWithAsset(ctx, &v1.File{Id: fileId, Filename: "test.txt"}, &v1.Asset{Id: uuid.New()})

	// Act (Connect to non-existent space)
	err := s.ConnectToSpace(ctx, fileId, uuid.New())

	// Assert
	assert.ErrorIs(t, err, service.ErrSpaceNotFound)
}
