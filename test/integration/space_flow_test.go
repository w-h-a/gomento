package integration

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAPI_Space(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	// Arrange
	client, baseURL, db, _ := setupIntegrationServer(t)

	projRsp := createJson(t, client, baseURL+"/api/v1/projects", map[string]any{"name": "Integration Proj"})
	projId := projRsp["id"].(string)
	require.NotEmpty(t, projId)

	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"project_id": projId})
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)

	sessRsp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]any{
		"project_id": projId,
		"space_id":   spaceId,
	})
	sessId := sessRsp["id"].(string)
	require.NotEmpty(t, sessId)

	expectedTaskId := uuid.New()

	_, err := db.Exec(`
		INSERT INTO tasks (id, session_id, task_order, status, is_thought, data, created_at, updated_at)
		VALUES ($1, $2, 1, 'pending', false, '{"task_description": "Injected Task"}', $3, $3)
	`, expectedTaskId, sessId, time.Now())
	require.NoError(t, err)

	otherSessRsp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]any{
		"project_id": projId,
		"space_id":   nil,
	})
	otherSessId := otherSessRsp["id"].(string)
	require.NotEmpty(t, otherSessId)

	_, err = db.Exec(`
		INSERT INTO tasks (id, session_id, task_order, status, is_thought, data, created_at, updated_at)
		VALUES ($1, $2, 1, 'pending', false, '{}', $3, $3)
	`, uuid.New(), otherSessId, time.Now())
	require.NoError(t, err)
}
