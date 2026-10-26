#!/bin/bash
echo "==> Triggering live data ingestion via FantasyLoL Admin API..."

# Send POST request to the admin update endpoint
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://localhost:8080/admin/stats/update)

if [ "$RESPONSE" -eq 200 ]; then
    echo "==> Success: Ingestion completed successfully!"
else
    echo "==> Error: Ingestion request failed with HTTP status $RESPONSE"
    echo "Check server terminal logs for detailed API error output."
fi