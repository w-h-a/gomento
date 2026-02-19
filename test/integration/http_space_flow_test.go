package integration

import (
	"bytes"
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
	v1space "github.com/w-h-a/gomento/internal/service/v1_space"
)

func TestAPI_Http_Space_Flow(t *testing.T) {
	if len(os.Getenv("INTEGRATION")) == 0 {
		t.Log("SKIPPING INTEGRATION TEST")
		return
	}

	client, baseURL, db, _ := setupHttpServer(t)

	// ==========================================
	// Scenario 1: Create Space & Session
	// ==========================================
	t.Log("Step 1: Creating Space & Session via HTTP")

	body, _ := json.Marshal(map[string]any{"name": "Frontend Space"})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/spaces", baseURL), bytes.NewBuffer(body))
	crtRsp, err := client.Do(req)
	require.NoError(t, err)
	defer crtRsp.Body.Close()
	assert.Equal(t, http.StatusCreated, crtRsp.StatusCode)

	var spaceRsp map[string]any
	json.NewDecoder(crtRsp.Body).Decode(&spaceRsp)
	spaceId := spaceRsp["id"].(string)
	require.NotEmpty(t, spaceId)
	assert.Equal(t, "Frontend Space", spaceRsp["name"])

	sessId := uuid.New()
	_, err = db.Exec("INSERT INTO sessions (id, space_id, created_at) VALUES ($1, $2, NOW())", sessId.String(), spaceId)
	require.NoError(t, err)

	// ==========================================
	// Scenario 2: Get Space
	// ==========================================
	t.Log("Step 2: Fetching Space Details via HTTP")

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s", baseURL, spaceId), nil)
	getRsp, err := client.Do(req)
	require.NoError(t, err)
	defer getRsp.Body.Close()
	assert.Equal(t, http.StatusOK, getRsp.StatusCode)

	var getRes map[string]any
	json.NewDecoder(getRsp.Body).Decode(&getRes)
	assert.Equal(t, spaceId, getRes["id"])

	// ==========================================
	// Scenario 3: List Spaces
	// ==========================================
	t.Log("Step 3: Listing Spaces via HTTP")

	// Create another one to ensure list works
	body, _ = json.Marshal(map[string]any{"name": "Backend Space"})
	req, _ = http.NewRequest("POST", fmt.Sprintf("%s/api/v1/spaces", baseURL), bytes.NewBuffer(body))
	crtRsp2, err := client.Do(req)
	require.NoError(t, err)
	defer crtRsp2.Body.Close()
	assert.Equal(t, http.StatusCreated, crtRsp2.StatusCode)

	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces", baseURL), nil)
	getRsp2, err := client.Do(req)
	require.NoError(t, err)
	defer getRsp2.Body.Close()
	assert.Equal(t, http.StatusOK, getRsp2.StatusCode)

	var listRsp v1space.ListSpacesOutput
	json.NewDecoder(getRsp2.Body).Decode(&listRsp)

	assert.GreaterOrEqual(t, len(listRsp.Items), 2)

	// ==========================================
	// Scenario 4: Semantic Search
	// ==========================================
	t.Log("Step 4: Searching Skills & Messages via HTTP")

	// Inject Data with Vectors manually
	vecNorth := make([]float32, 1536)
	vecNorth[0] = 1.0
	vecSouth := make([]float32, 1536)
	vecSouth[0] = -1.0
	vecEast := make([]float32, 1536)
	vecEast[0] = 0.5
	vecWest := make([]float32, 1536)
	vecWest[0] = -0.5

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

	// Inject Asset & File
	assetId := uuid.New()
	_, err = db.Exec(`
		INSERT INTO assets (id, container, path, etag, sha256, mime, size_bytes, created_at)
		VALUES ($1, 'test-bucket', 'docs/east.txt', 'etag', 'sha', 'text/plain', 100, NOW())
	`, assetId)
	require.NoError(t, err)

	fileId := uuid.New()

	_, err = db.Exec(`
		INSERT INTO files (id, space_id, asset_id, path, filename, embedding, created_at, updated_at)
		VALUES ($1, $2, $3, 'docs/east.txt', 'east.txt', $4, NOW(), NOW())
	`, fileId, spaceId, assetId, pgvector.NewVector(vecEast))
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO file_chunks (id, file_id, chunk_index, content, embedding, created_at)
		VALUES ($1, $2, 0, 'Go West Life is Peaceful There', $3, NOW())
	`, uuid.New(), fileId, pgvector.NewVector(vecWest))
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

	// Search Files
	reqFile, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/files?q=East", baseURL, spaceId), nil)
	rspFile, err := client.Do(reqFile)
	require.NoError(t, err)
	defer rspFile.Body.Close()
	assert.Equal(t, http.StatusOK, rspFile.StatusCode)

	var fileResults []v1.File
	json.NewDecoder(rspFile.Body).Decode(&fileResults)
	assert.GreaterOrEqual(t, len(fileResults), 1)
	assert.Equal(t, "east.txt", fileResults[0].Filename)

	// Search Chunks
	reqChunk, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/chunks?q=West", baseURL, spaceId), nil)
	rspChunk, err := client.Do(reqChunk)
	require.NoError(t, err)
	defer rspChunk.Body.Close()
	assert.Equal(t, http.StatusOK, rspChunk.StatusCode)

	var chunkResults []v1.MatchingChunk
	json.NewDecoder(rspChunk.Body).Decode(&chunkResults)
	assert.GreaterOrEqual(t, len(chunkResults), 1)
	assert.Contains(t, chunkResults[0].Chunk.Content, "Go West")

	// ==========================================
	// Scenario 5: Search with Limit
	// ==========================================
	t.Log("Step 5: Searching with limit param via HTTP")

	// Inject a second skill
	_, err = db.Exec(`
		INSERT INTO skills (id, space_id, trigger, sop, embedding, created_at)
		VALUES ($1, $2, 'Trigger East', 'SOP Content', $3, NOW())
	`, uuid.New(), spaceId, pgvector.NewVector(vecEast))
	require.NoError(t, err)

	// Search Skills with limit=1
	reqSkillLim, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/spaces/%s/skills?q=trigger&limit=1", baseURL, spaceId), nil)
	rspSkillLim, err := client.Do(reqSkillLim)
	require.NoError(t, err)
	defer rspSkillLim.Body.Close()
	assert.Equal(t, http.StatusOK, rspSkillLim.StatusCode)

	var skillLimResults []v1.Skill
	json.NewDecoder(rspSkillLim.Body).Decode(&skillLimResults)
	assert.Len(t, skillLimResults, 1)
}
