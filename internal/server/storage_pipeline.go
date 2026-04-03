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
// Pipeline operations
// -------------------------------------------------------------------------

// CreatePipeline validates uniqueness, sets server-assigned fields, and stores
// the pipeline. Returns AlreadyExists if a pipeline with the same name already
// exists.
func (s *Storage) CreatePipeline(ctx context.Context, parent, pipelineID string, pl *eventarcpb.Pipeline) (*eventarcpb.Pipeline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/pipelines/%s", parent, pipelineID)
	if _, exists := s.pipelines[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "Pipeline [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()
	etag := newEtag()

	stored := cloneProto(pl)
	stored.Name = name
	stored.Uid = uid
	stored.CreateTime = now
	stored.UpdateTime = now
	stored.Etag = etag

	s.pipelines[name] = stored
	return cloneProto(stored), nil
}

// GetPipeline returns the pipeline with the given full resource name.
// Returns NotFound if the pipeline does not exist.
func (s *Storage) GetPipeline(ctx context.Context, name string) (*eventarcpb.Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.pipelines[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Pipeline [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// UpdatePipeline applies the fields specified in mask to the stored pipeline
// and sets update_time and etag. Returns NotFound if the pipeline does not exist.
// Supported mask fields: labels, annotations, display_name, destinations,
// mediations, crypto_key_name, input_payload_format, logging_config, retry_policy.
func (s *Storage) UpdatePipeline(ctx context.Context, pl *eventarcpb.Pipeline, mask *fieldmaskpb.FieldMask) (*eventarcpb.Pipeline, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.pipelines[pl.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "Pipeline [%s] not found", pl.GetName())
	}

	if mask != nil {
		for _, path := range mask.GetPaths() {
			switch path {
			case "labels":
				stored.Labels = pl.GetLabels()
			case "annotations":
				stored.Annotations = pl.GetAnnotations()
			case "display_name":
				stored.DisplayName = pl.GetDisplayName()
			case "destinations":
				stored.Destinations = pl.GetDestinations()
			case "mediations":
				stored.Mediations = pl.GetMediations()
			case "crypto_key_name":
				stored.CryptoKeyName = pl.GetCryptoKeyName()
			case "input_payload_format":
				stored.InputPayloadFormat = pl.GetInputPayloadFormat()
			case "logging_config":
				stored.LoggingConfig = pl.GetLoggingConfig()
			case "retry_policy":
				stored.RetryPolicy = pl.GetRetryPolicy()
			}
		}
	} else {
		// No mask: update all mutable fields.
		stored.Labels = pl.GetLabels()
		stored.Annotations = pl.GetAnnotations()
		stored.DisplayName = pl.GetDisplayName()
		stored.Destinations = pl.GetDestinations()
		stored.Mediations = pl.GetMediations()
		stored.CryptoKeyName = pl.GetCryptoKeyName()
		stored.InputPayloadFormat = pl.GetInputPayloadFormat()
		stored.LoggingConfig = pl.GetLoggingConfig()
		stored.RetryPolicy = pl.GetRetryPolicy()
	}

	stored.UpdateTime = timestamppb.Now()
	stored.Etag = newEtag()
	return cloneProto(stored), nil
}

// DeletePipeline removes the pipeline with the given full resource name.
// Returns NotFound if the pipeline does not exist.
func (s *Storage) DeletePipeline(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.pipelines[name]; !exists {
		return status.Errorf(codes.NotFound, "Pipeline [%s] not found", name)
	}
	delete(s.pipelines, name)
	return nil
}

// ListPipelines returns pipelines under the given parent with integer-offset
// pagination. Results are sorted by name.
func (s *Storage) ListPipelines(ctx context.Context, parent string, pageSize int32, pageToken string) ([]*eventarcpb.Pipeline, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/pipelines/"
	var all []*eventarcpb.Pipeline
	for name, pl := range s.pipelines {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(pl))
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
// GoogleApiSource operations
// -------------------------------------------------------------------------

// CreateGoogleApiSource validates uniqueness, sets server-assigned fields, and
// stores the source. Returns AlreadyExists if a source with the same name
// already exists.
func (s *Storage) CreateGoogleApiSource(ctx context.Context, parent, sourceID string, src *eventarcpb.GoogleApiSource) (*eventarcpb.GoogleApiSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := fmt.Sprintf("%s/googleApiSources/%s", parent, sourceID)
	if _, exists := s.googleApiSources[name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "GoogleApiSource [%s] already exists", name)
	}

	now := timestamppb.Now()
	uid := newUID()
	etag := newEtag()

	stored := cloneProto(src)
	stored.Name = name
	stored.Uid = uid
	stored.CreateTime = now
	stored.UpdateTime = now
	stored.Etag = etag

	s.googleApiSources[name] = stored
	return cloneProto(stored), nil
}

// GetGoogleApiSource returns the source with the given full resource name.
// Returns NotFound if the source does not exist.
func (s *Storage) GetGoogleApiSource(ctx context.Context, name string) (*eventarcpb.GoogleApiSource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stored, exists := s.googleApiSources[name]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "GoogleApiSource [%s] not found", name)
	}
	return cloneProto(stored), nil
}

// UpdateGoogleApiSource applies the fields specified in mask to the stored
// source and sets update_time and etag. Returns NotFound if the source does not
// exist. Supported mask fields: labels, annotations, display_name, destination,
// crypto_key_name, logging_config.
func (s *Storage) UpdateGoogleApiSource(ctx context.Context, src *eventarcpb.GoogleApiSource, mask *fieldmaskpb.FieldMask) (*eventarcpb.GoogleApiSource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, exists := s.googleApiSources[src.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "GoogleApiSource [%s] not found", src.GetName())
	}

	if mask != nil {
		for _, path := range mask.GetPaths() {
			switch path {
			case "labels":
				stored.Labels = src.GetLabels()
			case "annotations":
				stored.Annotations = src.GetAnnotations()
			case "display_name":
				stored.DisplayName = src.GetDisplayName()
			case "destination":
				stored.Destination = src.GetDestination()
			case "crypto_key_name":
				stored.CryptoKeyName = src.GetCryptoKeyName()
			case "logging_config":
				stored.LoggingConfig = src.GetLoggingConfig()
			}
		}
	} else {
		// No mask: update all mutable fields.
		stored.Labels = src.GetLabels()
		stored.Annotations = src.GetAnnotations()
		stored.DisplayName = src.GetDisplayName()
		stored.Destination = src.GetDestination()
		stored.CryptoKeyName = src.GetCryptoKeyName()
		stored.LoggingConfig = src.GetLoggingConfig()
	}

	stored.UpdateTime = timestamppb.Now()
	stored.Etag = newEtag()
	return cloneProto(stored), nil
}

// DeleteGoogleApiSource removes the source with the given full resource name.
// Returns NotFound if the source does not exist.
func (s *Storage) DeleteGoogleApiSource(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.googleApiSources[name]; !exists {
		return status.Errorf(codes.NotFound, "GoogleApiSource [%s] not found", name)
	}
	delete(s.googleApiSources, name)
	return nil
}

// ListGoogleApiSources returns sources under the given parent with
// integer-offset pagination. Results are sorted by name.
func (s *Storage) ListGoogleApiSources(ctx context.Context, parent string, pageSize int32, pageToken string) ([]*eventarcpb.GoogleApiSource, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := parent + "/googleApiSources/"
	var all []*eventarcpb.GoogleApiSource
	for name, src := range s.googleApiSources {
		if strings.HasPrefix(name, prefix) {
			all = append(all, cloneProto(src))
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
