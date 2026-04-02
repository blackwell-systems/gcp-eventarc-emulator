// Package server provides an in-memory implementation of the GCP Eventarc API.
package server

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// defaultProviders is the set of well-known Eventarc providers seeded at startup.
var defaultProviders = []struct{ id, displayName string }{
	{"pubsub.googleapis.com", "Cloud Pub/Sub"},
	{"storage.googleapis.com", "Cloud Storage"},
	{"run.googleapis.com", "Cloud Run"},
	{"cloudfunctions.googleapis.com", "Cloud Functions"},
	{"cloudscheduler.googleapis.com", "Cloud Scheduler"},
}

// Storage is the in-memory store for Eventarc resources.
// All methods are thread-safe (sync.RWMutex internally).
type Storage struct {
	mu                   sync.RWMutex
	triggers             map[string]*eventarcpb.Trigger  // key: full resource name
	providers            map[string]*eventarcpb.Provider // key: full resource name, seeded at init
	channels             map[string]*eventarcpb.Channel
	channelConnections   map[string]*eventarcpb.ChannelConnection
	googleChannelConfigs map[string]*eventarcpb.GoogleChannelConfig
	messageBuses         map[string]*eventarcpb.MessageBus
	enrollments          map[string]*eventarcpb.Enrollment
	pipelines            map[string]*eventarcpb.Pipeline
	googleApiSources     map[string]*eventarcpb.GoogleApiSource
}

// NewStorage creates a new Storage instance seeded with default providers.
func NewStorage() *Storage {
	s := &Storage{
		triggers:             make(map[string]*eventarcpb.Trigger),
		providers:            make(map[string]*eventarcpb.Provider),
		channels:             make(map[string]*eventarcpb.Channel),
		channelConnections:   make(map[string]*eventarcpb.ChannelConnection),
		googleChannelConfigs: make(map[string]*eventarcpb.GoogleChannelConfig),
		messageBuses:         make(map[string]*eventarcpb.MessageBus),
		enrollments:          make(map[string]*eventarcpb.Enrollment),
		pipelines:            make(map[string]*eventarcpb.Pipeline),
		googleApiSources:     make(map[string]*eventarcpb.GoogleApiSource),
	}
	// Seed providers using a synthetic parent that allows wildcard matching.
	for _, p := range defaultProviders {
		name := fmt.Sprintf("projects/-/locations/-/providers/%s", p.id)
		s.providers[name] = &eventarcpb.Provider{
			Name:        name,
			DisplayName: p.displayName,
		}
	}
	return s
}

// -------------------------------------------------------------------------
// Trigger operations
// -------------------------------------------------------------------------

// CreateTrigger validates uniqueness, sets create/update time and uid, and
// stores the trigger. Returns AlreadyExists if a trigger with the same name
// already exists.
func (s *Storage) CreateTrigger(ctx context.Context, parent, triggerID string, trigger *eventarcpb.Trigger) (*eventarcpb.Trigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/triggers/%s", parent, triggerID)
	if _, exists := s.triggers[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "Trigger [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := fmt.Sprintf("%x", rand.Uint64())

	stored := cloneTrigger(trigger)
	stored.Name = name
	stored.Uid = uid
	stored.CreateTime = now
	stored.UpdateTime = now

	s.triggers[name] = stored
	return cloneTrigger(stored), nil
}

// GetTrigger returns the trigger with the given full resource name.
// Returns NotFound if the trigger does not exist.
func (s *Storage) GetTrigger(ctx context.Context, name string) (*eventarcpb.Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.triggers[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Trigger [%s] not found", name)
	}
	return cloneTrigger(stored), nil
}

// UpdateTrigger applies the fields specified in updateMask to the stored
// trigger and sets update_time. Returns NotFound if the trigger does not exist.
// Supported mask fields: labels, destination, event_filters, service_account, channel.
func (s *Storage) UpdateTrigger(ctx context.Context, trigger *eventarcpb.Trigger, updateMask *fieldmaskpb.FieldMask) (*eventarcpb.Trigger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.triggers[trigger.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Trigger [%s] not found", trigger.GetName())
	}

	// Apply update mask fields.
	if updateMask != nil {
		for _, path := range updateMask.GetPaths() {
			switch path {
			case "labels":
				stored.Labels = trigger.GetLabels()
			case "destination":
				stored.Destination = trigger.GetDestination()
			case "event_filters":
				stored.EventFilters = trigger.GetEventFilters()
			case "service_account":
				stored.ServiceAccount = trigger.GetServiceAccount()
			case "channel":
				stored.Channel = trigger.GetChannel()
			}
		}
	} else {
		// No mask: update all mutable fields.
		stored.Labels = trigger.GetLabels()
		stored.Destination = trigger.GetDestination()
		stored.EventFilters = trigger.GetEventFilters()
		stored.ServiceAccount = trigger.GetServiceAccount()
		stored.Channel = trigger.GetChannel()
	}

	stored.UpdateTime = timestamppb.Now()
	return cloneTrigger(stored), nil
}

// DeleteTrigger removes the trigger with the given full resource name.
// Returns NotFound if the trigger does not exist.
func (s *Storage) DeleteTrigger(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.triggers[name]; !exists {
		return status.Errorf(codes.NotFound, "Trigger [%s] not found", name)
	}
	delete(s.triggers, name)
	return nil
}

// ListTriggers returns triggers under the given parent, with optional sorting,
// filtering, and integer-offset pagination.
//
// orderBy values: "name" (default), "create_time desc".
// filter: simple "trigger_id=X" form; unparseable filters are silently ignored.
// pageToken: integer string offset into the sorted result set.
func (s *Storage) ListTriggers(ctx context.Context, parent string, pageSize int32, pageToken string, orderBy string, filter string) ([]*eventarcpb.Trigger, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/triggers/"
	var all []*eventarcpb.Trigger
	for name, t := range s.triggers {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneTrigger(t))
		}
	}

	// Apply simple filter.
	if filter != "" {
		all = applyTriggerFilter(all, filter)
	}

	// Sort.
	switch strings.TrimSpace(strings.ToLower(orderBy)) {
	case "create_time desc":
		sort.Slice(all, func(i, j int) bool {
			ti := all[i].GetCreateTime().AsTime()
			tj := all[j].GetCreateTime().AsTime()
			return ti.After(tj) // descending
		})
	default: // "name" or empty
		sort.Slice(all, func(i, j int) bool {
			return all[i].GetName() < all[j].GetName()
		})
	}

	// Paginate.
	startIdx := 0
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil {
			startIdx = n
		}
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	endIdx := startIdx + int(pageSize)
	if endIdx > len(all) {
		endIdx = len(all)
	}

	var results []*eventarcpb.Trigger
	if startIdx < len(all) {
		results = all[startIdx:endIdx]
	}

	nextToken := ""
	if endIdx < len(all) {
		nextToken = strconv.Itoa(endIdx)
	}
	return results, nextToken, nil
}

// ListAllTriggers returns all triggers under the given parent without pagination.
// Used by the router to match incoming events.
func (s *Storage) ListAllTriggers(ctx context.Context, parent string) ([]*eventarcpb.Trigger, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/triggers/"
	var all []*eventarcpb.Trigger
	for name, t := range s.triggers {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneTrigger(t))
		}
	}
	return all, nil
}

// -------------------------------------------------------------------------
// Provider operations
// -------------------------------------------------------------------------

// GetProvider returns a provider by full resource name.
// Because providers are seeded with a synthetic parent (projects/-/locations/-),
// this method matches by provider ID suffix so callers can use any project/location.
func (s *Storage) GetProvider(ctx context.Context, name string) (*eventarcpb.Provider, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Direct lookup first.
	if p, ok := s.providers[name]; ok {
		return cloneProvider(p, name), nil
	}

	// Match by provider ID suffix (e.g. "pubsub.googleapis.com").
	providerID := extractProviderID(name)
	if providerID == "" {
		return nil, status.Errorf(codes.NotFound, "Provider [%s] not found", name)
	}
	syntheticKey := fmt.Sprintf("projects/-/locations/-/providers/%s", providerID)
	if p, ok := s.providers[syntheticKey]; ok {
		return cloneProvider(p, name), nil
	}
	return nil, status.Errorf(codes.NotFound, "Provider [%s] not found", name)
}

// ListProviders returns providers under the given parent, with optional
// pagination and filtering. Providers are matched by ID regardless of
// project/location in the parent.
func (s *Storage) ListProviders(ctx context.Context, parent string, pageSize int32, pageToken string, filter string, orderBy string) ([]*eventarcpb.Provider, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect all providers, rewriting their names to match the requested parent.
	var all []*eventarcpb.Provider
	for key, p := range s.providers {
		providerID := extractProviderID(key)
		if providerID == "" {
			continue
		}
		realName := fmt.Sprintf("%s/providers/%s", parent, providerID)
		all = append(all, cloneProvider(p, realName))
	}

	// Sort by name for deterministic ordering.
	sort.Slice(all, func(i, j int) bool {
		return all[i].GetName() < all[j].GetName()
	})

	// Paginate.
	startIdx := 0
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil {
			startIdx = n
		}
	}
	if pageSize <= 0 {
		pageSize = 100
	}

	endIdx := startIdx + int(pageSize)
	if endIdx > len(all) {
		endIdx = len(all)
	}

	var results []*eventarcpb.Provider
	if startIdx < len(all) {
		results = all[startIdx:endIdx]
	}

	nextToken := ""
	if endIdx < len(all) {
		nextToken = strconv.Itoa(endIdx)
	}
	return results, nextToken, nil
}

// -------------------------------------------------------------------------
// Testing helpers
// -------------------------------------------------------------------------

// Clear removes all triggers from storage. Does not reset providers.
func (s *Storage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggers = make(map[string]*eventarcpb.Trigger)
}

// TriggerCount returns the number of triggers in storage.
func (s *Storage) TriggerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.triggers)
}

// -------------------------------------------------------------------------
// Internal helpers
// -------------------------------------------------------------------------

// cloneTrigger returns a deep copy of the trigger proto using proto.Clone.
func cloneTrigger(t *eventarcpb.Trigger) *eventarcpb.Trigger {
	if t == nil {
		return nil
	}
	return proto.Clone(t).(*eventarcpb.Trigger)
}

// cloneProvider returns a deep copy of p with Name overwritten to realName.
func cloneProvider(p *eventarcpb.Provider, realName string) *eventarcpb.Provider {
	if p == nil {
		return nil
	}
	c := proto.Clone(p).(*eventarcpb.Provider)
	c.Name = realName
	return c
}

// extractProviderID returns the last path segment of a provider resource name.
// e.g. "projects/p/locations/l/providers/pubsub.googleapis.com" → "pubsub.googleapis.com"
func extractProviderID(name string) string {
	const marker = "/providers/"
	idx := strings.LastIndex(name, marker)
	if idx < 0 {
		return ""
	}
	return name[idx+len(marker):]
}

// applyTriggerFilter applies a simple "trigger_id=X" filter to the trigger list.
// Unrecognised filter expressions are silently ignored (return all triggers).
func applyTriggerFilter(triggers []*eventarcpb.Trigger, filter string) []*eventarcpb.Trigger {
	filter = strings.TrimSpace(filter)
	const prefix = "trigger_id="
	if !strings.HasPrefix(filter, prefix) {
		return triggers
	}
	id := strings.TrimPrefix(filter, prefix)
	id = strings.Trim(id, `"'`)

	var out []*eventarcpb.Trigger
	for _, t := range triggers {
		// The trigger ID is the last segment of the full resource name.
		parts := strings.Split(t.GetName(), "/")
		if len(parts) > 0 && parts[len(parts)-1] == id {
			out = append(out, t)
		}
	}
	return out
}
