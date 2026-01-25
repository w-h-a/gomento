package unit

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	mockembedder "github.com/w-h-a/gomento/internal/client/embedder/mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestCreate_PersistsSpace(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()
	s := v1space.NewV1Service(p, e)
	ctx := context.Background()

	// Act
	space, err := s.Create(ctx, "DevOps Space")
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, space.Id)

	// Assert: State
	savedSpace, err := p.GetSpace(ctx, space.Id)
	assert.NoError(t, err)
	assert.NotNil(t, savedSpace)
	assert.Equal(t, "DevOps Space", savedSpace.Name)
}

func TestSearchSkills_OrchestratesVectorSearch(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()
	s := v1space.NewV1Service(p, e)
	ctx := context.Background()

	// Setup Data
	spaceId := uuid.New()
	expectedTrigger := "how to fix nginx"

	// Create a skill that should be found
	err := p.SaveSkill(ctx, &v1.Skill{
		Id:      uuid.New(),
		SpaceId: spaceId,
		Trigger: expectedTrigger,
		SOP:     "restart it",
	})
	require.NoError(t, err)

	// Create a noise skill in a different space
	err = p.SaveSkill(ctx, &v1.Skill{
		Id:      uuid.New(),
		SpaceId: uuid.New(),
		Trigger: "irrelevant",
	})
	require.NoError(t, err)

	// Act
	results, err := s.SearchSkills(ctx, spaceId, "nginx help")
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, expectedTrigger, results[0].Trigger)
	assert.Equal(t, "nginx help", e.Input())
}

func TestSearchMessages_OrchestratesVectorSearch(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()
	s := v1space.NewV1Service(p, e)
	ctx := context.Background()

	spaceId := uuid.New()

	// Setup Session & Message in the Space
	sess := &v1.Session{Id: uuid.New(), SpaceId: &spaceId}
	require.NoError(t, p.CreateSession(ctx, sess))

	msg := &v1.Message{
		Id:        uuid.New(),
		SessionId: sess.Id,
		Role:      "user",
		Parts:     []v1.Part{{Type: "text", Text: "relevant message"}},
	}
	require.NoError(t, p.CreateMessageWithAssets(ctx, msg, nil))

	// Act
	results, err := s.SearchMessages(ctx, spaceId, "find me")
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, msg.Id, results[0].Id)
	assert.Equal(t, "find me", e.Input())
}

func TestSearchFiles_OrchestratesVectorSearch(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()
	s := v1space.NewV1Service(p, e)
	ctx := context.Background()

	spaceId := uuid.New()

	// Setup: Create a file in the space
	fileId := uuid.New()
	err := p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		SpaceId:  &spaceId,
		Filename: "logs.txt",
		Path:     "var/logs",
	}, &v1.Asset{Id: uuid.New()})
	require.NoError(t, err)

	// Act
	results, err := s.SearchFiles(ctx, spaceId, "error logs")
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, fileId, results[0].Id)
	assert.Equal(t, "error logs", e.Input())
}

func TestSearchChunks_OrchestratesVectorSearch(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	e := mockembedder.NewEmbedder()
	s := v1space.NewV1Service(p, e)
	ctx := context.Background()

	spaceId := uuid.New()
	fileId := uuid.New()

	// Setup: Create a file and a chunk in the space
	err := p.UpsertFileWithAsset(ctx, &v1.File{
		Id:       fileId,
		SpaceId:  &spaceId,
		Filename: "manual.pdf",
	}, &v1.Asset{Id: uuid.New()})
	require.NoError(t, err)

	expectedChunk := v1.FileChunk{
		Id:         uuid.New(),
		FileId:     fileId,
		ChunkIndex: 0,
		Content:    "specific knowledge about deploying",
	}
	err = p.SaveFileChunks(ctx, fileId, []v1.FileChunk{expectedChunk})
	require.NoError(t, err)

	// Act
	results, err := s.SearchChunks(ctx, spaceId, "deploying help")
	require.NoError(t, err)

	// Assert
	assert.Len(t, results, 1)
	assert.Equal(t, expectedChunk.Id, results[0].Chunk.Id)
	assert.Equal(t, "specific knowledge about deploying", results[0].Chunk.Content)
	assert.Equal(t, "deploying help", e.Input())
}
