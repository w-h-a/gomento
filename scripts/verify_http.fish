#!/usr/bin/env fish

# CONFIGURATION
set BASE_URL "http://localhost:4000/api/v1/spaces"
set TRACE_ID "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0"
set SPAN_ID "abcdef1234567890"
set TRACEPARENT "00-$TRACE_ID-$SPAN_ID-01"

echo "🔍 Testing HTTP Trace Propagation..."
echo "🎯 Target: $BASE_URL"
echo "🆔 Trace ID: $TRACE_ID"

curl -v -s "$BASE_URL" \
  -H "Content-Type: application/json" \
  -H "traceparent: $TRACEPARENT"

echo -e "\n\n✅ Request Complete!"
echo "👉 Go check Jaeger for Trace ID: $TRACE_ID"
echo "   You should see a trace starting with 'HTTP GET' (or similar) followed by 'space.List'"