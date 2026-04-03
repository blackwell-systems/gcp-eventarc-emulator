#!/usr/bin/env bash
# GCP Eventarc Emulator — End-to-End Demo
#
# Prerequisites: docker compose up -d
#
# Demonstrates: trigger creation → event publishing → CloudEvent delivery
# in binary content mode (ce-* headers), matching real GCP Eventarc behavior.

set -euo pipefail

BASE_URL="http://localhost:8085"
PROJECT="demo-project"
LOCATION="us-central1"
API="$BASE_URL/v1/projects/$PROJECT/locations/$LOCATION"

GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
RESET='\033[0m'

step() { echo -e "\n${CYAN}▸ $1${RESET}"; }
ok()   { echo -e "${GREEN}  ✓ $1${RESET}"; }
info() { echo -e "${YELLOW}  $1${RESET}"; }

# ─── Wait for emulator ───────────────────────────────────────────────

step "Waiting for emulator to be ready..."
for i in $(seq 1 30); do
  if curl -sf "$API/triggers" > /dev/null 2>&1; then
    ok "Emulator is ready"
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Emulator not ready after 30s. Run: docker compose up -d"
    exit 1
  fi
  sleep 1
done

# ─── 1. List providers (seeded at startup) ───────────────────────────

step "List providers (seeded at startup)"
curl -s "$API/providers" | python3 -m json.tool
ok "Providers listed"

# ─── 2. Create a channel ─────────────────────────────────────────────

step "Create a channel"
curl -s -X POST "$API/channels?channelId=my-channel" \
  -H "Content-Type: application/json" \
  -d '{}' | python3 -m json.tool
ok "Channel created"

# ─── 3. Create a trigger ─────────────────────────────────────────────
#
# This trigger matches Pub/Sub events and routes them to the webhook
# receiver running at http://webhook:3000 inside the Docker network.

step "Create a trigger (routes Pub/Sub events → webhook receiver)"
curl -s -X POST "$API/triggers?triggerId=pubsub-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.pubsub.topic.v1.messagePublished"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://webhook:3000/events"}
    }
  }' | python3 -m json.tool
ok "Trigger created"

# ─── 4. Create a second trigger with a different event type ──────────

step "Create a second trigger (routes storage events → webhook)"
curl -s -X POST "$API/triggers?triggerId=storage-trigger" \
  -H "Content-Type: application/json" \
  -d '{
    "eventFilters": [
      {"attribute": "type", "value": "google.cloud.storage.object.v1.finalized"}
    ],
    "destination": {
      "httpEndpoint": {"uri": "http://webhook:3000/events"}
    }
  }' | python3 -m json.tool
ok "Storage trigger created"

# ─── 5. List triggers ────────────────────────────────────────────────

step "List all triggers"
curl -s "$API/triggers" | python3 -m json.tool
ok "Triggers listed"

# ─── 6. Publish a CloudEvent via the Publisher service ────────────────
#
# Uses the grpc-gateway REST endpoint for PublishEvents.
# The emulator will: match → route → HTTP POST to webhook in binary
# content mode (Ce-* headers).

step "Publish a Pub/Sub CloudEvent via REST"
info "POST $API/channels/my-channel:publishEvents"
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "$API/channels/my-channel:publishEvents" \
  -H "Content-Type: application/json" \
  -d '{
    "events": [
      {
        "@type": "type.googleapis.com/google.cloud.eventarc.publishing.v1.CloudEvent",
        "id": "evt-001",
        "source": "//pubsub.googleapis.com/projects/demo-project/topics/my-topic",
        "specVersion": "1.0",
        "type": "google.cloud.pubsub.topic.v1.messagePublished",
        "attributes": {
          "subject": {"ceString": "my-subject"},
          "time": {"ceTimestamp": "2026-04-02T12:00:00Z"}
        },
        "textData": "{\"subscription\":\"projects/demo-project/subscriptions/my-sub\",\"message\":{\"data\":\"SGVsbG8gZnJvbSBFdmVudGFyYyE=\"}}"
      }
    ]
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ "$HTTP_CODE" -eq 200 ]; then
  ok "Event published (HTTP $HTTP_CODE)"
  info "Check 'docker compose logs webhook' to see the delivered CloudEvent"
else
  echo "  Response ($HTTP_CODE): $BODY"
fi

# ─── 7. Create a message bus ─────────────────────────────────────────

step "Create a message bus"
curl -s -X POST "$API/messageBuses?messageBusId=my-bus" \
  -H "Content-Type: application/json" \
  -d '{}' | python3 -m json.tool
ok "Message bus created"

# ─── 8. Create a pipeline ────────────────────────────────────────────

step "Create a pipeline"
curl -s -X POST "$API/pipelines?pipelineId=my-pipeline" \
  -H "Content-Type: application/json" \
  -d '{
    "destinations": [
      {
        "httpEndpoint": {"uri": "http://webhook:3000/pipeline"}
      }
    ]
  }' | python3 -m json.tool
ok "Pipeline created"

# ─── 9. Create an enrollment ─────────────────────────────────────────

step "Create an enrollment"
curl -s -X POST "$API/enrollments?enrollmentId=my-enrollment" \
  -H "Content-Type: application/json" \
  -d '{
    "celMatch": "message.type == '\''google.cloud.pubsub.topic.v1.messagePublished'\''",
    "messageBus": "projects/demo-project/locations/us-central1/messageBuses/my-bus",
    "destination": "projects/demo-project/locations/us-central1/pipelines/my-pipeline"
  }' | python3 -m json.tool
ok "Enrollment created"

# ─── 10. Get Google Channel Config ───────────────────────────────────

step "Get Google Channel Config (singleton)"
curl -s "$API/googleChannelConfig" | python3 -m json.tool
ok "Google Channel Config retrieved"

# ─── Summary ──────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}══════════════════════════════════════════════════${RESET}"
echo -e "${GREEN}  Demo complete!${RESET}"
echo -e "${GREEN}══════════════════════════════════════════════════${RESET}"
echo ""
echo "  Resources created:"
echo "    • 1 channel          (my-channel)"
echo "    • 2 triggers         (pubsub-trigger, storage-trigger)"
echo "    • 1 message bus      (my-bus)"
echo "    • 1 pipeline         (my-pipeline)"
echo "    • 1 enrollment       (my-enrollment)"
echo "    • 1 CloudEvent published and routed"
echo ""
echo "  See delivered event:"
echo "    docker compose logs webhook"
echo ""
echo "  Tear down:"
echo "    docker compose down"
echo ""
