package unit

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1mock "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestCreate_PersistsSpaceLinkedToProject(t *testing.T) {
	// Arrange
	p := v1mock.NewV1Persister()
	s := v1space.NewV1Service(p)
	projectId := uuid.New()
	ctx := context.Background()

	// Act
	space, err := s.Create(ctx, projectId, "DevOps Space")
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, space.Id)
	assert.Equal(t, projectId, space.ProjectId)

	// Assert: State
	savedSpace := p.Spaces()[space.Id]
	assert.NotNil(t, savedSpace)
	assert.Equal(t, "DevOps Space", savedSpace.Name)
}
