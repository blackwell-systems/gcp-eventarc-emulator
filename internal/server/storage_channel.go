// Package server provides Channel, ChannelConnection, and GoogleChannelConfig
// in-memory storage methods on the shared Storage type.
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
// Channel operations
// -------------------------------------------------------------------------

// CreateChannel validates uniqueness, sets server-assigned fields, and stores
// the channel. Returns AlreadyExists if a channel with the same name exists.
func (s *Storage) CreateChannel(ctx context.Context, parent, channelID string, ch *eventarcpb.Channel) (*eventarcpb.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/channels/%s", parent, channelID)
	if _, exists := s.channels[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "Channel [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()

	stored := cloneProto(ch)
	stored.Name = name
	stored.Uid = uid
	stored.CreateTime = now
	stored.UpdateTime = now
	stored.State = eventarcpb.Channel_ACTIVE

	s.channels[name] = stored
	return cloneProto(stored), nil
}

// GetChannel returns the channel with the given full resource name.
// Returns NotFound if the channel does not exist.
func (s *Storage) GetChannel(ctx context.Context, name string) (*eventarcpb.Channel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.channels[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Channel [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// GetChannelExists reports whether a channel with the given full resource
// name exists in storage. Used by the publisher to validate channels
// before routing events.
func (s *Storage) GetChannelExists(ctx context.Context, name string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.channels[name]
	return ok, nil
}

// UpdateChannel applies the fields specified in mask to the stored channel and
// sets update_time. Returns NotFound if the channel does not exist.
// Supported mask fields: labels, crypto_key_name, state.
func (s *Storage) UpdateChannel(ctx context.Context, ch *eventarcpb.Channel, mask *fieldmaskpb.FieldMask) (*eventarcpb.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.channels[ch.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Channel [%s] not found", ch.GetName())
	}

	if mask != nil {
		for _, path := range mask.GetPaths() {
			switch path {
			case "labels":
				stored.Labels = ch.GetLabels()
			case "crypto_key_name":
				stored.CryptoKeyName = ch.GetCryptoKeyName()
			case "state":
				stored.State = ch.GetState()
			}
		}
	} else {
		stored.Labels = ch.GetLabels()
		stored.CryptoKeyName = ch.GetCryptoKeyName()
		stored.State = ch.GetState()
	}

	stored.UpdateTime = timestamppb.Now()
	return cloneProto(stored), nil
}

// DeleteChannel removes the channel with the given full resource name.
// Returns NotFound if the channel does not exist.
func (s *Storage) DeleteChannel(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.channels[name]; !exists {
		return status.Errorf(codes.NotFound, "Channel [%s] not found", name)
	}
	delete(s.channels, name)
	return nil
}

// ListChannels returns channels under the given parent with integer-offset pagination.
func (s *Storage) ListChannels(ctx context.Context, parent string, pageSize int32, pageToken string) ([]*eventarcpb.Channel, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/channels/"
	var all []*eventarcpb.Channel
	for name, ch := range s.channels {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(ch))
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].GetName() < all[j].GetName()
	})

	page, nextToken, err := PaginatePage(all, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}
	return page, nextToken, nil
}

// -------------------------------------------------------------------------
// ChannelConnection operations
// -------------------------------------------------------------------------

// CreateChannelConnection validates uniqueness, sets server-assigned fields,
// and stores the connection. Returns AlreadyExists if a connection with the
// same name exists.
func (s *Storage) CreateChannelConnection(ctx context.Context, parent, connID string, conn *eventarcpb.ChannelConnection) (*eventarcpb.ChannelConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/channelConnections/%s", parent, connID)
	if _, exists := s.channelConnections[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "ChannelConnection [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()

	stored := cloneProto(conn)
	stored.Name = name
	stored.Uid = uid
	stored.CreateTime = now
	stored.UpdateTime = now

	s.channelConnections[name] = stored
	return cloneProto(stored), nil
}

// GetChannelConnection returns the channel connection with the given full
// resource name. Returns NotFound if the connection does not exist.
func (s *Storage) GetChannelConnection(ctx context.Context, name string) (*eventarcpb.ChannelConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.channelConnections[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "ChannelConnection [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// DeleteChannelConnection removes the channel connection with the given full
// resource name. Returns NotFound if the connection does not exist.
func (s *Storage) DeleteChannelConnection(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.channelConnections[name]; !exists {
		return status.Errorf(codes.NotFound, "ChannelConnection [%s] not found", name)
	}
	delete(s.channelConnections, name)
	return nil
}

// ListChannelConnections returns channel connections under the given parent
// with integer-offset pagination.
func (s *Storage) ListChannelConnections(ctx context.Context, parent string, pageSize int32, pageToken string) ([]*eventarcpb.ChannelConnection, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/channelConnections/"
	var all []*eventarcpb.ChannelConnection
	for name, cc := range s.channelConnections {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(cc))
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].GetName() < all[j].GetName()
	})

	page, nextToken, err := PaginatePage(all, pageToken, pageSize)
	if err != nil {
		return nil, "", err
	}
	return page, nextToken, nil
}

// -------------------------------------------------------------------------
// GoogleChannelConfig operations
// -------------------------------------------------------------------------

// GetGoogleChannelConfig returns the config for the given full resource name.
// If no config has been stored yet, returns a zero-value config (never NotFound).
func (s *Storage) GetGoogleChannelConfig(ctx context.Context, name string) (*eventarcpb.GoogleChannelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if stored, exists := s.googleChannelConfigs[name]; exists {
		return cloneProto(stored), nil
	}
	return &eventarcpb.GoogleChannelConfig{
		Name:       name,
		UpdateTime: timestamppb.Now(),
	}, nil
}

// UpdateGoogleChannelConfig applies the fields specified in mask to the stored
// config and sets update_time. If no config exists, creates one from the
// provided cfg. Supported mask fields: crypto_key_name, labels.
func (s *Storage) UpdateGoogleChannelConfig(ctx context.Context, cfg *eventarcpb.GoogleChannelConfig, mask *fieldmaskpb.FieldMask) (*eventarcpb.GoogleChannelConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := cfg.GetName()
	stored, exists := s.googleChannelConfigs[name]
	if !exists {
		stored = &eventarcpb.GoogleChannelConfig{Name: name}
	}

	if mask != nil {
		for _, path := range mask.GetPaths() {
			switch path {
			case "crypto_key_name":
				stored.CryptoKeyName = cfg.GetCryptoKeyName()
			case "labels":
				stored.Labels = cfg.GetLabels()
			}
		}
	} else {
		stored.CryptoKeyName = cfg.GetCryptoKeyName()
		stored.Labels = cfg.GetLabels()
	}

	stored.UpdateTime = timestamppb.Now()
	s.googleChannelConfigs[name] = stored
	return cloneProto(stored), nil
}

