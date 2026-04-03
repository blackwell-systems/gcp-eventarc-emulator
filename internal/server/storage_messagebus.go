// Package server provides an in-memory implementation of the GCP Eventarc API.
package server

import (
	"context"
	"fmt"
	"sort"
	"strings"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// -------------------------------------------------------------------------
// MessageBus operations
// -------------------------------------------------------------------------

// CreateMessageBus validates uniqueness, sets metadata fields, and stores the
// message bus. Returns AlreadyExists if a message bus with the same name
// already exists.
func (s *Storage) CreateMessageBus(ctx context.Context, parent, busID string, mb *eventarcpb.MessageBus) (*eventarcpb.MessageBus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/messageBuses/%s", parent, busID)
	if _, exists := s.messageBuses[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "MessageBus [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()
	etag := newEtag()

	stored := cloneProto(mb)
	stored.Name = name
	stored.Uid = uid
	stored.Etag = etag
	stored.CreateTime = now
	stored.UpdateTime = now

	s.messageBuses[name] = stored
	return cloneProto(stored), nil
}

// GetMessageBus returns the message bus with the given full resource name.
// Returns NotFound if the message bus does not exist.
func (s *Storage) GetMessageBus(ctx context.Context, name string) (*eventarcpb.MessageBus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.messageBuses[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "MessageBus [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// UpdateMessageBus applies the fields specified in updateMask to the stored
// message bus and sets update_time and etag. Returns NotFound if not found.
// Supported mask fields: labels, annotations, display_name, crypto_key_name,
// logging_config.
func (s *Storage) UpdateMessageBus(ctx context.Context, mb *eventarcpb.MessageBus, updateMask *fieldmaskpb.FieldMask) (*eventarcpb.MessageBus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.messageBuses[mb.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "MessageBus [%s] not found", mb.GetName())
	}

	if updateMask != nil {
		for _, path := range updateMask.GetPaths() {
			switch path {
			case "*":
				stored.Labels = mb.GetLabels()
				stored.Annotations = mb.GetAnnotations()
				stored.DisplayName = mb.GetDisplayName()
				stored.CryptoKeyName = mb.GetCryptoKeyName()
				stored.LoggingConfig = mb.GetLoggingConfig()
			case "labels":
				stored.Labels = mb.GetLabels()
			case "annotations":
				stored.Annotations = mb.GetAnnotations()
			case "display_name":
				stored.DisplayName = mb.GetDisplayName()
			case "crypto_key_name":
				stored.CryptoKeyName = mb.GetCryptoKeyName()
			case "logging_config":
				stored.LoggingConfig = mb.GetLoggingConfig()
			}
		}
	} else {
		// No mask: update all mutable fields.
		stored.Labels = mb.GetLabels()
		stored.Annotations = mb.GetAnnotations()
		stored.DisplayName = mb.GetDisplayName()
		stored.CryptoKeyName = mb.GetCryptoKeyName()
		stored.LoggingConfig = mb.GetLoggingConfig()
	}

	stored.UpdateTime = timestamppb.Now()
	stored.Etag = newEtag()
	return cloneProto(stored), nil
}

// DeleteMessageBus removes the message bus with the given full resource name.
// Returns NotFound if the message bus does not exist.
func (s *Storage) DeleteMessageBus(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.messageBuses[name]; !exists {
		return status.Errorf(codes.NotFound, "MessageBus [%s] not found", name)
	}
	delete(s.messageBuses, name)
	return nil
}

// ListMessageBuses returns message buses under the given parent with
// integer-offset pagination. Results are sorted by name.
func (s *Storage) ListMessageBuses(ctx context.Context, parent string, pageSize int32, pageToken string, orderBy string) ([]*eventarcpb.MessageBus, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/messageBuses/"
	var all []*eventarcpb.MessageBus
	for name, mb := range s.messageBuses {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(mb))
		}
	}

	switch strings.TrimSpace(strings.ToLower(orderBy)) {
	case "create_time desc":
		sort.Slice(all, func(i, j int) bool {
			return all[i].GetCreateTime().AsTime().After(all[j].GetCreateTime().AsTime())
		})
	default:
		sort.Slice(all, func(i, j int) bool {
			return all[i].GetName() < all[j].GetName()
		})
	}

	page, nextToken, err := PaginatePage(all, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}
	return page, nextToken, nil
}

// ListMessageBusEnrollments returns the resource names ([]string) of
// enrollments whose MessageBus field equals the given bus name, with
// integer-offset pagination.
func (s *Storage) ListMessageBusEnrollments(ctx context.Context, busName string, pageSize int32, pageToken string) ([]string, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var all []string
	for name, en := range s.enrollments {
		if en.GetMessageBus() == busName {
			all = append(all, name)
		}
	}

	sort.Strings(all)

	page, nextToken, err := PaginatePage[string](all, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}
	return page, nextToken, nil
}

// -------------------------------------------------------------------------
// Enrollment operations
// -------------------------------------------------------------------------

// CreateEnrollment validates uniqueness, sets metadata fields, and stores
// the enrollment. Returns AlreadyExists if an enrollment with the same name
// already exists.
func (s *Storage) CreateEnrollment(ctx context.Context, parent, enrollmentID string, en *eventarcpb.Enrollment) (*eventarcpb.Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/enrollments/%s", parent, enrollmentID)
	if _, exists := s.enrollments[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "Enrollment [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()
	etag := newEtag()

	stored := cloneProto(en)
	stored.Name = name
	stored.Uid = uid
	stored.Etag = etag
	stored.CreateTime = now
	stored.UpdateTime = now

	s.enrollments[name] = stored
	return cloneProto(stored), nil
}

// GetEnrollment returns the enrollment with the given full resource name.
// Returns NotFound if the enrollment does not exist.
func (s *Storage) GetEnrollment(ctx context.Context, name string) (*eventarcpb.Enrollment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.enrollments[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Enrollment [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// UpdateEnrollment applies the fields specified in updateMask to the stored
// enrollment and sets update_time and etag. Returns NotFound if not found.
// Supported mask fields: labels, annotations, display_name, cel_match,
// message_bus, destination.
func (s *Storage) UpdateEnrollment(ctx context.Context, en *eventarcpb.Enrollment, updateMask *fieldmaskpb.FieldMask) (*eventarcpb.Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.enrollments[en.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Enrollment [%s] not found", en.GetName())
	}

	if updateMask != nil {
		for _, path := range updateMask.GetPaths() {
			switch path {
			case "*":
				stored.Labels = en.GetLabels()
				stored.Annotations = en.GetAnnotations()
				stored.DisplayName = en.GetDisplayName()
				stored.CelMatch = en.GetCelMatch()
				stored.Destination = en.GetDestination()
				// Note: message_bus is intentionally NOT set here (immutable field).
			case "labels":
				stored.Labels = en.GetLabels()
			case "annotations":
				stored.Annotations = en.GetAnnotations()
			case "display_name":
				stored.DisplayName = en.GetDisplayName()
			case "cel_match":
				stored.CelMatch = en.GetCelMatch()
			case "message_bus":
				return nil, status.Errorf(codes.InvalidArgument,
					"field message_bus is immutable and cannot be updated")
			case "destination":
				stored.Destination = en.GetDestination()
			}
		}
	} else {
		// No mask: update all mutable fields.
		// Note: message_bus is intentionally NOT set here (immutable field).
		stored.Labels = en.GetLabels()
		stored.Annotations = en.GetAnnotations()
		stored.DisplayName = en.GetDisplayName()
		stored.CelMatch = en.GetCelMatch()
		stored.Destination = en.GetDestination()
	}

	stored.UpdateTime = timestamppb.Now()
	stored.Etag = newEtag()
	return cloneProto(stored), nil
}

// DeleteEnrollment removes the enrollment with the given full resource name.
// Returns NotFound if the enrollment does not exist.
func (s *Storage) DeleteEnrollment(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.enrollments[name]; !exists {
		return status.Errorf(codes.NotFound, "Enrollment [%s] not found", name)
	}
	delete(s.enrollments, name)
	return nil
}

// ListEnrollments returns enrollments under the given parent with
// integer-offset pagination. Results are sorted by name.
func (s *Storage) ListEnrollments(ctx context.Context, parent string, pageSize int32, pageToken string, orderBy string) ([]*eventarcpb.Enrollment, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/enrollments/"
	var all []*eventarcpb.Enrollment
	for name, en := range s.enrollments {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(en))
		}
	}

	switch strings.TrimSpace(strings.ToLower(orderBy)) {
	case "create_time desc":
		sort.Slice(all, func(i, j int) bool {
			return all[i].GetCreateTime().AsTime().After(all[j].GetCreateTime().AsTime())
		})
	default:
		sort.Slice(all, func(i, j int) bool {
			return all[i].GetName() < all[j].GetName()
		})
	}

	page, nextToken, err := PaginatePage(all, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}
	return page, nextToken, nil
}
