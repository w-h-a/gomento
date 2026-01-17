#!/usr/bin/env fish

# CONFIGURATION
set BASE_URL "http://localhost:4001/mcp"

echo "🚀 Starting Realistic DevOps Workflow Simulation..."

# --- STEP 1: INITIALIZE MCP SESSION ---
echo -e "\n🔌 1. Initializing MCP Connection..."
curl -s -D headers.txt -o /dev/null -X POST "$BASE_URL" \
    -H "Content-Type: application/json" \
    -d '{ "jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "1.0", "clientInfo": {"name": "fish-client", "version": "1.0"}} }'

set SESSION_ID (grep -i "^Mcp-Session-Id:" headers.txt | cut -d ':' -f 2 | tr -d '[:space:]')
if test -z "$SESSION_ID"
    echo "❌ Failed to get Session ID"
    exit 1
end
echo "✅ MCP Session Established: $SESSION_ID"


# --- STEP 2: CREATE CHAT SESSION ---
# Trace: 1111...
set TP "00-11111111111111111111111111111111-abcdef1234567890-01"
echo -e "\n💬 2. Creating New Chat Session..."
echo "👉 Trace: $TP"

set RESP (curl -s -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 2, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"create_session\",
      \"arguments\": {
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }")

set CHAT_ID (echo $RESP | grep -oE '"id":"[a-f0-9-]+"' | head -n 1 | cut -d '"' -f 4)
echo "✅ Chat Session ID: $CHAT_ID"


# --- STEP 3: USER ASKS "HOW TO DEPLOY" ---
# Trace: 2222...
set TP "00-22222222222222222222222222222222-abcdef1234567890-01"
echo -e "\n🗣️ 3. User: 'How do I deploy to production?'"
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 3, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"add_message\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"role\": \"user\",
        \"parts\": [{\"type\": \"text\", \"text\": \"How do I deploy to production?\"}],
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 4: AGENT PROVIDES ANSWER (TAGGING STRATEGY) ---
# Trace: 3333...
set TP "00-33333333333333333333333333333333-abcdef1234567890-01"
echo -e "\n🤖 4. Agent: 'Use GitHub tags (rc for staging, semver for prod)...'"
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 4, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"add_message\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"role\": \"assistant\",
        \"parts\": [{\"type\": \"text\", \"text\": \"You need to create a GitHub tag. Use a candidate tag (e.g., v1.0.0-rc1) for staging. Once verified, push a clean semver tag (v1.0.0) to trigger the production workflow.\"}],
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 5: CREATE 'DEVOPS' SPACE ---
# Trace: 4444...
set TP "00-44444444444444444444444444444444-abcdef1234567890-01"
echo -e "\n📁 5. Creating Space 'DevOps Engineering'..."
echo "👉 Trace: $TP"

set RESP (curl -s -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 5, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"create_space\",
      \"arguments\": {
        \"name\": \"DevOps Engineering\",
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }")

set SPACE_ID (echo $RESP | grep -oE '"id":"[a-f0-9-]+"' | head -n 1 | cut -d '"' -f 4)
echo "✅ Space ID: $SPACE_ID"


# --- STEP 6: CONNECT SESSION TO SPACE ---
# Trace: 5555...
set TP "00-55555555555555555555555555555555-abcdef1234567890-01"
echo -e "\n🔗 6. Connecting Chat to Space..."
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 6, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"connect_session_to_space\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"space_id\": \"$SPACE_ID\",
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 7: EXTRACT TASKS (FIRST PASS) ---
# Trace: 6666...
set TP "00-66666666666666666666666666666666-abcdef1234567890-01"
echo -e "\n🧠 7. Triggering Task Extraction (Analysis of Steps)..."
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 7, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"extract_tasks\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 8: USER CONFIRMS SUCCESS ---
# Trace: 7777...
set TP "00-77777777777777777777777777777777-abcdef1234567890-01"
echo -e "\n🗣️ 8. User: 'Okay, that worked. Staging is deployed.'"
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 8, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"add_message\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"role\": \"user\",
        \"parts\": [{\"type\": \"text\", \"text\": \"Okay, that worked. Staging is deployed.\"}],
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 9: EXTRACT TASKS (SECOND PASS - UPDATE STATUS) ---
# Trace: 8888...
set TP "00-88888888888888888888888888888888-abcdef1234567890-01"
echo -e "\n🧠 9. Triggering Task Extraction (Update Status)..."
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 9, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"extract_tasks\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"


# --- STEP 10: DISTILL SKILL ---
# Trace: 9999...
set TP "00-99999999999999999999999999999999-abcdef1234567890-01"
echo -e "\n💎 10. Distilling Skill for Space..."
echo "👉 Trace: $TP"

curl -s -o /dev/null -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 10, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"distill_skill\",
      \"arguments\": {
        \"session_id\": \"$CHAT_ID\",
        \"_meta\": { \"traceparent\": \"$TP\" }
      }
    }
  }"

echo "⏳ Waiting 5s for background job..."
sleep 10

# --- STEP 11: VERIFY SKILL (SEMANTIC SEARCH) ---
echo -e "\n🔎 11. Verifying Skill via Semantic Search..."
set SEARCH_RESP (curl -s -X POST "$BASE_URL" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -H "Content-Type: application/json" \
  -d "{
    \"jsonrpc\": \"2.0\", \"id\": 11, \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"search_skills\",
      \"arguments\": {
        \"space_id\": \"$SPACE_ID\",
        \"query\": \"how to deploy to production\"
      }
    }
  }")

# Check if we got a hit
echo $SEARCH_RESP | grep "GitHub tag" > /dev/null
if test $status -eq 0
    echo "✅ Success! Found skill containing 'GitHub tag'"
else
    echo "❌ Failed. Skill not found in search results."
    echo "Response: $SEARCH_RESP"
end

# Cleanup
rm headers.txt
echo -e "\n\n🎉 Workflow Complete!"
echo "👉 Check Jaeger for trace IDs: 1111..., 2222..., 3333..., etc."