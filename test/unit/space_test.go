package unit

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
	v1mock "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1mockpersister "github.com/w-h-a/gomento/internal/client/persister/v1_mock"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestCreate_PersistsSpaceLinkedToProject(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

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
	savedSpace, err := p.GetSpace(ctx, space.Id)
	assert.NoError(t, err)
	assert.NotNil(t, savedSpace)
	assert.Equal(t, "DevOps Space", savedSpace.Name)
}

func TestListTasks_ReturnsAllTasksForSpace(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1space.NewV1Service(p)

	ctx := context.Background()

	spaceId := uuid.New()
	p.CreateSpace(ctx, &v1.Space{Id: spaceId, Name: "Target Space"})

	sessionInSpace := uuid.New()
	p.CreateSession(ctx, &v1.Session{Id: sessionInSpace, SpaceId: &spaceId})

	otherSpaceId := uuid.New()
	sessionOutside := uuid.New()
	p.CreateSession(ctx, &v1.Session{Id: sessionOutside, SpaceId: &otherSpaceId})

	data1, _ := json.Marshal(map[string]string{"task_description": "Task In Space"})
	_, err := p.InsertTask(ctx, sessionInSpace, 0, data1, "pending")
	require.NoError(t, err)

	data2, _ := json.Marshal(map[string]string{"task_description": "Done Task"})
	_, err = p.InsertTask(ctx, sessionInSpace, 1, data2, "success")
	require.NoError(t, err)

	data3, _ := json.Marshal(map[string]string{"task_description": "Task Outside"})
	_, err = p.InsertTask(ctx, sessionOutside, 0, data3, "pending")
	require.NoError(t, err)

	// Act
	out, err := s.ListTasks(ctx, v1space.ListTasksInput{
		SpaceId: spaceId,
	})
	require.NoError(t, err)

	// Assert
	assert.Len(t, out.Items, 2)
	statuses := []string{out.Items[0].Status, out.Items[1].Status}
	assert.Contains(t, statuses, "pending")
	assert.Contains(t, statuses, "success")
}

func TestListTasks_FiltersByStatus(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) > 0 {
		t.Log("SKIPPING UNIT TEST")
		return
	}

	// Arrange
	p := v1mockpersister.NewV1Persister()
	s := v1space.NewV1Service(p)

	ctx := context.Background()

	spaceId := uuid.New()
	p.CreateSpace(ctx, &v1.Space{Id: spaceId, Name: "Target Space"})

	sessionInSpace := uuid.New()
	p.CreateSession(ctx, &v1.Session{Id: sessionInSpace, SpaceId: &spaceId})

	data1, _ := json.Marshal(map[string]string{"task_description": "Task In Space"})
	p.InsertTask(ctx, sessionInSpace, 0, data1, "pending")

	data2, _ := json.Marshal(map[string]string{"task_description": "Done Task"})
	p.InsertTask(ctx, sessionInSpace, 1, data2, "success")

	// Act
	status := "success"
	out, err := s.ListTasks(ctx, v1space.ListTasksInput{
		SpaceId: spaceId,
		Status:  &status,
	})
	require.NoError(t, err)

	// Assert
	assert.Len(t, out.Items, 1)
	assert.Equal(t, "success", out.Items[0].Status)
	var dataMap map[string]string
	_ = json.Unmarshal(out.Items[0].Data, &dataMap)
	assert.Equal(t, "Done Task", dataMap["task_description"])
}
