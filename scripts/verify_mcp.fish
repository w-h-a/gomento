#!/usr/bin/env fish

# CONFIGURATION
set BASE_URL "http://localhost:4001/mcp"
set TRACE_ID "a0b0c0d0e0f0a0b0c0d0e0f0a0b0c0d0"
set SPAN_ID "1234567890abcdef"
set TRACEPARENT "00-$TRACE_ID-$SPAN_ID-01"

echo "🔍 1. Initializing Session..."

# 1. Initialize to get the Session ID
curl -s -D headers.txt -o /dev/null -X POST "$BASE_URL" \
    -H "Content-Type: application/json" \
    -d '{
        "jsonrpc": "2.0", 
        "id": 1, 
        "method": "initialize", 
        "params": {"protocolVersion": "1.0"}
    }'

# 2. Extract Mcp-Session-Id from the headers
set SESSION_ID (grep -i "^Mcp-Session-Id:" headers.txt | cut -d ':' -f 2 | tr -d '[:space:]')

if test -z "$SESSION_ID"
    echo "❌ ERROR: Could not find 'Mcp-Session-Id' header."
    echo "--- Headers Received ---"
    cat headers.txt
    rm headers.txt
    exit 1
end

echo "✅ Found Session ID: $SESSION_ID"

echo "🚀 2. Sending Trace Verification Request..."

# 3. Send the Tool Call using the Header
curl -s -X POST "$BASE_URL" \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: $SESSION_ID" \
  -d "{
    \"jsonrpc\": \"2.0\",
    \"method\": \"tools/call\",
    \"params\": {
      \"name\": \"list_spaces\",
      \"arguments\": {
        \"_meta\": {
          \"traceparent\": \"$TRACEPARENT\"
        }
      }
    },
    \"id\": 2
  }"

echo -e "\n\n✅ Request Sent! Check Jaeger for Trace ID: $TRACE_ID"

# Cleanup
rm headers.txt