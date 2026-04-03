package server

import (
	"context"
	"testing"
	"time"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// -------------------------------------------------------------------------
// Pipeline tests
// -------------------------------------------------------------------------

// TestStorageCreatePipeline_Success verifies that a new pipeline is stored and
// returned with server-assigned fields (name, uid, create_time, update_time, etag).
func TestStorageCreatePipeline_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	pl := &eventarcpb.Pipeline{
		DisplayName: "My Pipeline",
	}

	got, err := s.CreatePipeline(ctx, testParent, "my-pipeline", pl)
	if err != nil {
		t.Fatalf("CreatePipeline: unexpected error: %v", err)
	}

	wantName := testParent + "/pipelines/my-pipeline"
	if got.GetName() != wantName {
		t.Errorf("Name = %q, want %q", got.GetName(), wantName)
	}
	if got.GetUid() == "" {
		t.Error("Uid should not be empty after create")
	}
	if got.GetCreateTime() == nil {
		t.Error("CreateTime should not be nil after create")
	}
	if got.GetUpdateTime() == nil {
		t.Error("UpdateTime should not be nil after create")
	}
	if got.GetEtag() == "" {
		t.Error("Etag should not be empty after create")
	}
	if got.GetDisplayName() != "My Pipeline" {
		t.Errorf("DisplayName = %q, want %q", got.GetDisplayName(), "My Pipeline")
	}
}

// TestStorageCreatePipeline_AlreadyExists verifies that creating a pipeline with
// a duplicate name returns an AlreadyExists error.
func TestStorageCreatePipeline_AlreadyExists(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	pl := &eventarcpb.Pipeline{}
	if _, err := s.CreatePipeline(ctx, testParent, "dup-pipeline", pl); err != nil {
		t.Fatalf("first CreatePipeline: unexpected error: %v", err)
	}

	_, err := s.CreatePipeline(ctx, testParent, "dup-pipeline", pl)
	if err == nil {
		t.Fatal("expected AlreadyExists error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("error code = %v, want AlreadyExists", err)
	}
}

// TestStorageGetPipeline_NotFound verifies that looking up a non-existent
// pipeline returns a NotFound error.
func TestStorageGetPipeline_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.GetPipeline(ctx, testParent+"/pipelines/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageDeletePipeline_NotFound verifies that deleting a non-existent
// pipeline returns a NotFound error.
func TestStorageDeletePipeline_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	err := s.DeletePipeline(ctx, testParent+"/pipelines/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageListPipelines_Pagination creates 5 pipelines and verifies that
// listing them with pageSize=2 returns pages of 2, 2, and 1.
func TestStorageListPipelines_Pagination(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	ids := []string{"pipeline-a", "pipeline-b", "pipeline-c", "pipeline-d", "pipeline-e"}
	for _, id := range ids {
		if _, err := s.CreatePipeline(ctx, testParent, id, &eventarcpb.Pipeline{}); err != nil {
			t.Fatalf("CreatePipeline(%q): %v", id, err)
		}
	}

	var allGot []string
	token := ""
	pages := 0
	for {
		results, next, err := s.ListPipelines(ctx, testParent, 2, token, "")
		if err != nil {
			t.Fatalf("ListPipelines: %v", err)
		}
		pages++
		for _, pl := range results {
			allGot = append(allGot, pl.GetName())
		}
		if next == "" {
			break
		}
		token = next
		if pages > 10 {
			t.Fatal("too many pages, infinite loop suspected")
		}
	}

	if len(allGot) != 5 {
		t.Errorf("total results = %d, want 5", len(allGot))
	}
	if pages != 3 {
		t.Errorf("pages = %d, want 3", pages)
	}
}

// -------------------------------------------------------------------------
// GoogleApiSource tests
// -------------------------------------------------------------------------

// TestStorageCreateGoogleApiSource_Success verifies that a new GoogleApiSource
// is stored and returned with server-assigned fields.
func TestStorageCreateGoogleApiSource_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	src := &eventarcpb.GoogleApiSource{
		DisplayName: "My Source",
		Destination: "projects/p/locations/l/messageBuses/mb",
	}

	got, err := s.CreateGoogleApiSource(ctx, testParent, "my-source", src)
	if err != nil {
		t.Fatalf("CreateGoogleApiSource: unexpected error: %v", err)
	}

	wantName := testParent + "/googleApiSources/my-source"
	if got.GetName() != wantName {
		t.Errorf("Name = %q, want %q", got.GetName(), wantName)
	}
	if got.GetUid() == "" {
		t.Error("Uid should not be empty after create")
	}
	if got.GetCreateTime() == nil {
		t.Error("CreateTime should not be nil after create")
	}
	if got.GetUpdateTime() == nil {
		t.Error("UpdateTime should not be nil after create")
	}
	if got.GetEtag() == "" {
		t.Error("Etag should not be empty after create")
	}
	if got.GetDisplayName() != "My Source" {
		t.Errorf("DisplayName = %q, want %q", got.GetDisplayName(), "My Source")
	}
	if got.GetDestination() != src.GetDestination() {
		t.Errorf("Destination = %q, want %q", got.GetDestination(), src.GetDestination())
	}
}

// TestStorageGetGoogleApiSource_NotFound verifies that looking up a
// non-existent source returns a NotFound error.
func TestStorageGetGoogleApiSource_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.GetGoogleApiSource(ctx, testParent+"/googleApiSources/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestUpdatePipeline_WildcardMask verifies that a wildcard mask ("*") updates
// all mutable fields of a Pipeline.
func TestUpdatePipeline_WildcardMask(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	created, err := s.CreatePipeline(ctx, testParent, "pl-wc", &eventarcpb.Pipeline{
		DisplayName: "Original",
		Labels:      map[string]string{"stage": "dev"},
	})
	if err != nil {
		t.Fatalf("CreatePipeline: %v", err)
	}

	updated, err := s.UpdatePipeline(ctx, &eventarcpb.Pipeline{
		Name:        created.GetName(),
		DisplayName: "Updated",
		Labels:      map[string]string{"stage": "prod"},
	}, &fieldmaskpb.FieldMask{Paths: []string{"*"}})
	if err != nil {
		t.Fatalf("UpdatePipeline wildcard: %v", err)
	}

	if updated.GetDisplayName() != "Updated" {
		t.Errorf("DisplayName = %q, want %q", updated.GetDisplayName(), "Updated")
	}
	if updated.GetLabels()["stage"] != "prod" {
		t.Errorf("Labels[stage] = %q, want %q", updated.GetLabels()["stage"], "prod")
	}
}

// TestUpdateGoogleApiSource_WildcardMask verifies that a wildcard mask ("*")
// updates all mutable fields of a GoogleApiSource.
func TestUpdateGoogleApiSource_WildcardMask(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	src := &eventarcpb.GoogleApiSource{
		DisplayName: "Original",
		Destination: testParent + "/messageBuses/mb",
	}
	created, err := s.CreateGoogleApiSource(ctx, testParent, "src-wc", src)
	if err != nil {
		t.Fatalf("CreateGoogleApiSource: %v", err)
	}

	newDest := testParent + "/messageBuses/mb-new"
	updated, err := s.UpdateGoogleApiSource(ctx, &eventarcpb.GoogleApiSource{
		Name:        created.GetName(),
		DisplayName: "Updated",
		Destination: newDest,
		Labels:      map[string]string{"key": "val"},
	}, &fieldmaskpb.FieldMask{Paths: []string{"*"}})
	if err != nil {
		t.Fatalf("UpdateGoogleApiSource wildcard: %v", err)
	}

	if updated.GetDisplayName() != "Updated" {
		t.Errorf("DisplayName = %q, want %q", updated.GetDisplayName(), "Updated")
	}
	if updated.GetDestination() != newDest {
		t.Errorf("Destination = %q, want %q", updated.GetDestination(), newDest)
	}
	if updated.GetLabels()["key"] != "val" {
		t.Errorf("Labels[key] = %q, want %q", updated.GetLabels()["key"], "val")
	}
}

// TestStorageUpdateGoogleApiSource_Success verifies that updating display_name
// bumps UpdateTime and preserves other fields.
func TestStorageUpdateGoogleApiSource_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	src := &eventarcpb.GoogleApiSource{
		DisplayName: "Original Name",
		Destination: "projects/p/locations/l/messageBuses/mb",
	}

	created, err := s.CreateGoogleApiSource(ctx, testParent, "update-me", src)
	if err != nil {
		t.Fatalf("CreateGoogleApiSource: %v", err)
	}

	// Sleep briefly so UpdateTime is guaranteed to be after CreateTime.
	time.Sleep(time.Millisecond)

	updated, err := s.UpdateGoogleApiSource(ctx, &eventarcpb.GoogleApiSource{
		Name:        created.GetName(),
		DisplayName: "Updated Name",
	}, &fieldmaskpb.FieldMask{Paths: []string{"display_name"}})
	if err != nil {
		t.Fatalf("UpdateGoogleApiSource: %v", err)
	}

	if updated.GetDisplayName() != "Updated Name" {
		t.Errorf("DisplayName = %q, want %q", updated.GetDisplayName(), "Updated Name")
	}
	if !updated.GetUpdateTime().AsTime().After(created.GetUpdateTime().AsTime()) {
		t.Errorf("UpdateTime was not bumped: created=%v updated=%v",
			created.GetUpdateTime().AsTime(), updated.GetUpdateTime().AsTime())
	}
	// Destination should be unchanged (not in mask).
	if updated.GetDestination() != src.GetDestination() {
		t.Errorf("Destination = %q, want %q", updated.GetDestination(), src.GetDestination())
	}
}
