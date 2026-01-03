package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
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

	// Act
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/tasks", baseURL, spaceId), nil)
	rsp, err := client.Do(req)
	require.NoError(t, err)
	defer rsp.Body.Close()

	// Assert
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	var out v1space.ListTasksOutput
	err = json.NewDecoder(rsp.Body).Decode(&out)
	assert.NoError(t, err)

	assert.Len(t, out.Items, 1)
	assert.Equal(t, expectedTaskId.String(), out.Items[0].Id.String())
	assert.Equal(t, "pending", out.Items[0].Status)

	var dataMap map[string]string
	err = json.Unmarshal(out.Items[0].Data, &dataMap)
	assert.NoError(t, err)
	assert.Equal(t, "Injected Task", dataMap["task_description"])
}
