package unit

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1mock "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1project "github.com/w-h-a/gomento/internal/service/v1_project"
)

func TestCreate_PersistsProject(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mock.NewV1Persister()
	s := v1project.NewV1Service(p)
	ctx := context.Background()

	// Act
	proj, err := s.Create(ctx, "Test Project")
	require.NoError(t, err)

	// Assert: Behavior
	assert.NotNil(t, proj.Id)
	assert.Equal(t, "Test Project", proj.Name)

	// Assert: State
	savedProj := p.Projects()[proj.Id]
	assert.NotNil(t, savedProj, "Project should be persisted in the DB")
	assert.Equal(t, proj.Name, savedProj.Name)
}
