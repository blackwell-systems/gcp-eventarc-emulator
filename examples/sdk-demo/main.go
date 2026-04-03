// SDK Demo — GCP Eventarc Emulator
//
// This program uses the official cloud.google.com/go/eventarc SDK to interact
// with the emulator. No GCP credentials required. No application-level code
// changes from what you'd write targeting real GCP — only the connection setup
// differs (insecure gRPC instead of default credentials).
//
// Usage:
//
//	docker compose up -d        # start emulator + webhook receiver
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	eventarc "cloud.google.com/go/eventarc/apiv1"
	"cloud.google.com/go/eventarc/apiv1/eventarcpb"
	publishing "cloud.google.com/go/eventarc/publishing/apiv1"
	"cloud.google.com/go/eventarc/publishing/apiv1/publishingpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	project  = "my-project"
	location = "us-central1"
)

func main() {
	emulatorAddr := envOr("EVENTARC_EMULATOR_HOST", "localhost:9085")
	webhookURI := envOr("WEBHOOK_URI", "http://webhook:3000/events")

	ctx := context.Background()

	// ── Connect to the emulator ──────────────────────────────────────────────
	//
	// This is the only setup change vs. real GCP: insecure gRPC to localhost
	// instead of default Application Default Credentials.
	//
	// In production you would use:
	//   client, err := eventarc.NewClient(ctx)
	//
	conn, err := grpc.NewClient(emulatorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	must(err, "dial emulator")

	client, err := eventarc.NewClient(ctx, option.WithGRPCConn(conn))
	must(err, "create eventarc client")
	defer client.Close()

	pubClient, err := publishing.NewPublisherClient(ctx, option.WithGRPCConn(conn))
	must(err, "create publisher client")
	defer pubClient.Close()

	parent := fmt.Sprintf("projects/%s/locations/%s", project, location)

	// ── 1. List providers ────────────────────────────────────────────────────

	fmt.Println("\n── 1. List providers (seeded at startup)")
	providerIter := client.ListProviders(ctx, &eventarcpb.ListProvidersRequest{Parent: parent})
	count := 0
	for {
		p, err := providerIter.Next()
		if err == iterator.Done {
			break
		}
		must(err, "list providers")
		fmt.Printf("   provider: %s\n", p.GetName())
		count++
	}
	fmt.Printf("   → %d providers\n", count)

	// ── 2. Get a specific provider ───────────────────────────────────────────

	fmt.Println("\n── 2. Get provider: pubsub")
	provider, err := client.GetProvider(ctx, &eventarcpb.GetProviderRequest{
		Name: fmt.Sprintf("%s/providers/pubsub.googleapis.com", parent),
	})
	must(err, "get provider")
	fmt.Printf("   name:        %s\n", provider.GetName())
	fmt.Printf("   displayName: %s\n", provider.GetDisplayName())

	// ── 3. Create a trigger ──────────────────────────────────────────────────

	fmt.Println("\n── 3. Create trigger")
	triggerID := fmt.Sprintf("sdk-trigger-%d", time.Now().Unix())
	op, err := client.CreateTrigger(ctx, &eventarcpb.CreateTriggerRequest{
		Parent:    parent,
		TriggerId: triggerID,
		Trigger: &eventarcpb.Trigger{
			EventFilters: []*eventarcpb.EventFilter{
				{
					Attribute: "type",
					Value:     "google.cloud.pubsub.topic.v1.messagePublished",
				},
			},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_HttpEndpoint{
					HttpEndpoint: &eventarcpb.HttpEndpoint{Uri: webhookURI},
				},
			},
		},
	})
	must(err, "create trigger LRO")

	trigger, err := op.Wait(ctx)
	must(err, "wait for trigger creation")
	fmt.Printf("   created: %s\n", trigger.GetName())
	fmt.Printf("   uid:     %s\n", trigger.GetUid())

	// ── 4. Get the trigger back ──────────────────────────────────────────────

	fmt.Println("\n── 4. Get trigger")
	got, err := client.GetTrigger(ctx, &eventarcpb.GetTriggerRequest{
		Name: trigger.GetName(),
	})
	must(err, "get trigger")
	fmt.Printf("   name:        %s\n", got.GetName())
	fmt.Printf("   destination: %s\n", got.GetDestination().GetHttpEndpoint().GetUri())

	// ── 5. List triggers ─────────────────────────────────────────────────────

	fmt.Println("\n── 5. List triggers")
	triggerIter := client.ListTriggers(ctx, &eventarcpb.ListTriggersRequest{Parent: parent})
	for {
		t, err := triggerIter.Next()
		if err == iterator.Done {
			break
		}
		must(err, "list triggers")
		fmt.Printf("   trigger: %s\n", t.GetName())
	}

	// ── 6. Update the trigger ────────────────────────────────────────────────

	fmt.Println("\n── 6. Update trigger (add label)")
	updateOp, err := client.UpdateTrigger(ctx, &eventarcpb.UpdateTriggerRequest{
		Trigger: &eventarcpb.Trigger{
			Name:   trigger.GetName(),
			Labels: map[string]string{"env": "local", "demo": "sdk"},
			EventFilters: []*eventarcpb.EventFilter{
				{Attribute: "type", Value: "google.cloud.pubsub.topic.v1.messagePublished"},
			},
			Destination: &eventarcpb.Destination{
				Descriptor_: &eventarcpb.Destination_HttpEndpoint{
					HttpEndpoint: &eventarcpb.HttpEndpoint{Uri: webhookURI},
				},
			},
		},
	})
	must(err, "update trigger LRO")
	updated, err := updateOp.Wait(ctx)
	must(err, "wait for update")
	fmt.Printf("   labels: %v\n", updated.GetLabels())

	// ── 7. Publish a CloudEvent via the Publisher service ────────────────────
	//
	// Uses the real cloud.google.com/go/eventarc/publishing/apiv1 SDK client —
	// the same client library used with production GCP Eventarc.

	fmt.Println("\n── 7. Publish CloudEvent via Publisher SDK")

	ce := &publishingpb.CloudEvent{
		Id:          fmt.Sprintf("sdk-evt-%d", time.Now().UnixNano()),
		Source:      fmt.Sprintf("//pubsub.googleapis.com/projects/%s/topics/my-topic", project),
		SpecVersion: "1.0",
		Type:        "google.cloud.pubsub.topic.v1.messagePublished",
		Attributes: map[string]*publishingpb.CloudEvent_CloudEventAttributeValue{
			"subject": {
				Attr: &publishingpb.CloudEvent_CloudEventAttributeValue_CeString{
					CeString: "my-subject",
				},
			},
		},
		Data: &publishingpb.CloudEvent_TextData{
			TextData: `{"subscription":"projects/my-project/subscriptions/my-sub","message":{"data":"SGVsbG8gZnJvbSB0aGUgR0NQIEV2ZW50YXJjIFNESyE="}}`,
		},
	}

	ceAny, err := anypb.New(ce)
	must(err, "marshal CloudEvent to Any")

	_, err = pubClient.PublishEvents(ctx, &publishingpb.PublishEventsRequest{
		Channel: fmt.Sprintf("%s/channels/my-channel", parent),
		Events:  []*anypb.Any{ceAny},
	})
	must(err, "publish events")
	fmt.Printf("   published: %s (type: %s)\n", ce.GetId(), ce.GetType())
	fmt.Println("   → event routed to trigger destination")

	// ── 8. Delete the trigger ────────────────────────────────────────────────

	fmt.Println("\n── 8. Delete trigger")
	deleteOp, err := client.DeleteTrigger(ctx, &eventarcpb.DeleteTriggerRequest{
		Name: trigger.GetName(),
	})
	must(err, "delete trigger LRO")
	_, err = deleteOp.Wait(ctx)
	must(err, "wait for delete")
	fmt.Printf("   deleted: %s\n", trigger.GetName())

	fmt.Println("\n✓ SDK demo complete")
	fmt.Println("  Run: docker compose logs webhook")
	fmt.Println("  to see the delivered CloudEvent with Ce-* headers")
	fmt.Println()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func must(err error, msg string) {
	if err != nil {
		log.Fatalf("ERROR %s: %v", msg, err)
	}
}
