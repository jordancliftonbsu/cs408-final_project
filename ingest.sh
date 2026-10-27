#!/bin/bash
#
# ingest.sh — Trigger a live Leaguepedia data ingestion run via the Admin API.
#
# The ingestion runs synchronously inside the server request handler, so the
# curl call below blocks until the entire run finishes. To give visibility
# while we wait, this script streams the server's live logs (via journalctl)
# in the background and prints elapsed-time heartbeats.
#
# Environment overrides:
#   INGEST_URL     full URL to POST to   (default http://localhost:8080/admin/stats/update)
#   SERVICE_NAME   systemd unit to tail  (default fantasylol)

# NOTE: deliberately NOT using `set -e`/`-o pipefail`. The background log
# pipeline (journalctl | sed) can exit non-zero when we kill it, and we do not
# want that to abort the script or mask curl's real exit code.
set -u

INGEST_URL="${INGEST_URL:-http://localhost:8080/admin/stats/update}"
SERVICE_NAME="${SERVICE_NAME:-fantasylol}"

# --- Up-front warning --------------------------------------------------------
echo "=================================================================="
echo "==> Triggering live data ingestion via FantasyLoL Admin API..."
echo "    Target: ${INGEST_URL}"
echo ""
echo "    WARNING: This may take SEVERAL MINUTES to complete."
echo "    The Leaguepedia Cargo API is rate-limited (a ~2s pause between"
echo "    every query, plus retry/backoff pacing), so a full multi-region"
echo "    sync can run for many minutes. Please leave this running and do"
echo "    not interrupt it — progress is streamed below as it happens."
echo "=================================================================="
echo ""

START_TS=$(date +%s)

LOG_PID=""
HEARTBEAT_PID=""

# Clean up background helpers on any exit (normal, error, or Ctrl+C).
cleanup() {
    [ -n "${HEARTBEAT_PID}" ] && kill "${HEARTBEAT_PID}" >/dev/null 2>&1
    [ -n "${LOG_PID}" ] && kill "${LOG_PID}" >/dev/null 2>&1
    return 0
}
trap cleanup EXIT

# --- Background: stream server logs so we can see progress -------------------
# We only attempt this if journalctl exists, the service is present, AND we can
# use sudo non-interactively (sudo -n). This avoids a sudo password prompt
# fighting the terminal for stdin, which previously caused the run to abort.
if command -v journalctl >/dev/null 2>&1 && systemctl status "${SERVICE_NAME}" >/dev/null 2>&1; then
    if sudo -n true >/dev/null 2>&1; then
        echo "==> Streaming live server logs from '${SERVICE_NAME}':"
        echo "------------------------------------------------------------------"
        # </dev/null detaches it from the terminal's stdin so it can never
        # consume our input or trigger a prompt.
        sudo -n journalctl -fu "${SERVICE_NAME}" --since now --no-pager </dev/null 2>/dev/null \
            | sed -u 's/^/    [server] /' &
        LOG_PID=$!
    else
        echo "==> (Log streaming needs passwordless sudo, which isn't available."
        echo "    The ingestion will still run. To watch progress in another"
        echo "    terminal:  sudo journalctl -fu ${SERVICE_NAME})"
    fi
else
    echo "==> (Live log streaming unavailable — journalctl or the"
    echo "    '${SERVICE_NAME}' service was not found. The request still runs;"
    echo "    check the server's own logs for detailed output.)"
fi

# --- Background: elapsed-time heartbeat --------------------------------------
(
    n=0
    while true; do
        sleep 30
        n=$((n + 30))
        printf '    [ingest.sh] still working... %dm%02ds elapsed\n' "$((n / 60))" "$((n % 60))"
    done
) </dev/null &
HEARTBEAT_PID=$!

# --- Fire the request (blocks until ingestion finishes) ----------------------
echo ""
echo "==> Request sent. Waiting for ingestion to finish..."
echo ""

RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" --max-time 3600 -X POST "${INGEST_URL}")
CURL_EXIT=$?

# Stop background helpers before printing the final result.
cleanup
trap - EXIT

END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))
ELAPSED_FMT=$(printf '%dm%02ds' "$((ELAPSED / 60))" "$((ELAPSED % 60))")

echo ""
echo "=================================================================="

if [ "${CURL_EXIT}" -eq 0 ] && [ "${RESPONSE}" = "200" ]; then
    echo "==> Success: Ingestion completed successfully! (took ${ELAPSED_FMT})"
    echo "=================================================================="
    exit 0
fi

# --- Error reporting ---------------------------------------------------------
case "${CURL_EXIT}" in
    0)
        echo "==> Error: Ingestion request failed with HTTP status ${RESPONSE}"
        echo "    Elapsed: ${ELAPSED_FMT}."
        echo "    The server was reached but returned an error. Check logs:"
        echo "      sudo journalctl -u ${SERVICE_NAME} -n 80 --no-pager"
        ;;
    2)
        echo "==> Interrupted (curl exit 130/2): the run was stopped before it"
        echo "    finished (Ctrl+C or a signal). Elapsed: ${ELAPSED_FMT}."
        echo "    Nothing was advanced; you can safely re-run ./ingest.sh."
        ;;
    7)
        echo "==> Error: Could not connect to the server (curl exit 7)."
        echo "    Is the '${SERVICE_NAME}' service running and listening at"
        echo "    ${INGEST_URL}?  Check:  sudo systemctl status ${SERVICE_NAME}"
        ;;
    28)
        echo "==> Error: Request timed out after 1 hour (curl exit 28)."
        echo "    Elapsed: ${ELAPSED_FMT}. The ingestion may still be running"
        echo "    server-side; check:  sudo journalctl -fu ${SERVICE_NAME}"
        ;;
    130)
        echo "==> Interrupted (Ctrl+C). Elapsed: ${ELAPSED_FMT}."
        echo "    Nothing was advanced; you can safely re-run ./ingest.sh."
        ;;
    *)
        echo "==> Error: Request failed (curl exit ${CURL_EXIT})."
        echo "    Elapsed: ${ELAPSED_FMT}. Is the service running at ${INGEST_URL}?"
        ;;
esac
echo "=================================================================="
exit 1
