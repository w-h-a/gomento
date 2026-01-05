package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/w-h-a/gomento/api/domain/v1"
)

func TestAPI_Space_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, _ := setupIntegrationServer(t)

	// ==========================================
	// Scenario 1: Create Space & Session
	// ==========================================
	t.Log("Step 1: Creating Space & Session")

	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"name": "Frontend Space"})
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)
	assert.Equal(t, "Frontend Space", spaceRsp["name"])

	sessRsp := createJson(t, client, baseURL+"/api/v1/sessions", map[string]any{"space_id": spaceId})
	sessId := sessRsp["id"].(string)
	require.NotEmpty(t, sessId)
	assert.Equal(t, spaceId, sessRsp["space_id"])

	// ==========================================
	// Scenario 2: Get Space
	// ==========================================
	t.Log("Step 2: Fetching Space Details")

	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s", baseURL, spaceId), nil)
	rsp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	var getRsp map[string]any
	json.NewDecoder(rsp.Body).Decode(&getRsp)
	assert.Equal(t, spaceId, getRsp["id"])

	// ==========================================
	// Scenario 3: List Spaces
	// ==========================================
	t.Log("Step 3: Listing Spaces")

	// Create another one to ensure list works
	createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"name": "Backend Space"})

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces", baseURL), nil)
	rsp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rsp.StatusCode)

	var listRsp struct {
		Items []map[string]any `json:"items"`
	}
	json.NewDecoder(rsp.Body).Decode(&listRsp)

	assert.GreaterOrEqual(t, len(listRsp.Items), 2)

	// ==========================================
	// Scenario 4: Semantic Search
	// ==========================================
	t.Log("Step 4: Searching Skills & Messages")

	// Inject Data with Vectors manually
	vecNorth := make([]float32, 1536)
	vecNorth[0] = 1.0
	vecSouth := make([]float32, 1536)
	vecSouth[0] = -1.0

	// Inject Skill
	_, err = db.Exec(`
		INSERT INTO skills (id, space_id, trigger, sop, embedding, created_at)
		VALUES ($1, $2, 'Trigger North', 'SOP Content', $3, NOW())
	`, uuid.New(), spaceId, pgvector.NewVector(vecNorth))
	require.NoError(t, err)

	// Inject Message
	_, err = db.Exec(`
		INSERT INTO messages (id, session_id, role, parts, embedding, created_at)
		VALUES ($1, $2, 'user', '[{"type":"text","text":"Go South"}]', $3, NOW())
	`, uuid.New(), sessId, pgvector.NewVector(vecSouth))
	require.NoError(t, err)

	// Search Skills
	reqSkill, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/skills?q=North", baseURL, spaceId), nil)
	rspSkill, err := client.Do(reqSkill)
	require.NoError(t, err)
	defer rspSkill.Body.Close()
	assert.Equal(t, http.StatusOK, rspSkill.StatusCode)

	var skillResults []v1.Skill
	json.NewDecoder(rspSkill.Body).Decode(&skillResults)
	assert.GreaterOrEqual(t, len(skillResults), 1)

	// Search Messages
	reqMsg, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/messages?q=South", baseURL, spaceId), nil)
	rspMsg, err := client.Do(reqMsg)
	require.NoError(t, err)
	defer rspMsg.Body.Close()
	assert.Equal(t, http.StatusOK, rspMsg.StatusCode)

	var msgResults []v1.Message
	json.NewDecoder(rspMsg.Body).Decode(&msgResults)
	assert.GreaterOrEqual(t, len(msgResults), 1)
}
