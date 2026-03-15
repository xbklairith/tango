#!/bin/bash
# Mock agent: reads env vars, calls API to mark issue done.
# Used by TestE2E_GoldenAgentJourney.

echo "mock-agent: starting"
echo "mock-agent: ARI_API_URL=${ARI_API_URL}"
echo "mock-agent: ARI_TASK_ID=${ARI_TASK_ID}"
echo "mock-agent: ARI_API_KEY length=${#ARI_API_KEY}"

# Retry with backoff to handle timing issues
for i in 1 2 3; do
  RESPONSE=$(curl -s -w "\n%{http_code}" -X PATCH "${ARI_API_URL}/api/agent/me/task" \
    -H "Authorization: Bearer ${ARI_API_KEY}" \
    -H "Content-Type: application/json" \
    -d "{\"issueId\": \"${ARI_TASK_ID}\", \"status\": \"done\"}" 2>&1)

  HTTP_CODE=$(echo "$RESPONSE" | tail -1)
  BODY=$(echo "$RESPONSE" | head -n -1)

  echo "mock-agent: attempt $i: HTTP $HTTP_CODE: $BODY"

  if [ "$HTTP_CODE" = "200" ]; then
    echo "mock-agent: done"
    exit 0
  fi

  sleep 1
done

echo "mock-agent: FAILED after 3 attempts"
exit 1
