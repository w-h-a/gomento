package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI_Space_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, _, _ := setupIntegrationServer(t)

	// ==========================================
	// Scenario 1: Create Space
	// ==========================================
	t.Log("Step 1: Creating Space")

	spaceRsp := createJson(t, client, baseURL+"/api/v1/spaces", map[string]any{"name": "Frontend Space"})
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)
	assert.Equal(t, "Frontend Space", spaceRsp["name"])

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
}
