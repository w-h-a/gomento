package unit

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1mock "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestCreate_PersistsSpace(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mock.NewV1Persister()
	s := v1space.NewV1Service(p)
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
