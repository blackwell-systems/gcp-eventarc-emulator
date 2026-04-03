package server

import (
	"context"
	"testing"

	eventarcpb "cloud.google.com/go/eventarc/apiv1/eventarcpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestStorageCreateChannel_Success verifies that a new channel is stored and
// returned with server-assigned fields (name, uid, create_time, update_time).
func TestStorageCreateChannel_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	ch := &eventarcpb.Channel{}
	got, err := s.CreateChannel(ctx, testParent, "my-channel", ch)
	if err != nil {
		t.Fatalf("CreateChannel: unexpected error: %v", err)
	}
	wantName := testParent + "/channels/my-channel"
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
}

// TestStorageCreateChannel_AlreadyExists verifies that creating a channel with
// a duplicate name returns an AlreadyExists error.
func TestStorageCreateChannel_AlreadyExists(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	ch := &eventarcpb.Channel{}
	if _, err := s.CreateChannel(ctx, testParent, "dup-channel", ch); err != nil {
		t.Fatalf("first CreateChannel: unexpected error: %v", err)
	}

	_, err := s.CreateChannel(ctx, testParent, "dup-channel", ch)
	if err == nil {
		t.Fatal("expected AlreadyExists error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("error code = %v, want AlreadyExists", err)
	}
}

// TestStorageGetChannel_NotFound verifies that looking up a non-existent channel
// returns a NotFound error.
func TestStorageGetChannel_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.GetChannel(ctx, testParent+"/channels/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageDeleteChannel_NotFound verifies that deleting a non-existent channel
// returns a NotFound error.
func TestStorageDeleteChannel_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	err := s.DeleteChannel(ctx, testParent+"/channels/ghost")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageListChannels_Pagination verifies that pageSize and pageToken
// correctly partition the full result set.
func TestStorageListChannels_Pagination(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Create 5 channels.
	for i := 0; i < 5; i++ {
		id := "channel-" + string(rune('a'+i))
		if _, err := s.CreateChannel(ctx, testParent, id, &eventarcpb.Channel{}); err != nil {
			t.Fatalf("CreateChannel %s: %v", id, err)
		}
	}

	// Page 1: first 2.
	page1, nextToken, err := s.ListChannels(ctx, testParent, 2, "")
	if err != nil {
		t.Fatalf("ListChannels page1: %v", err)
	}
	if len(page1) != 2 {
		t.Errorf("page1 len = %d, want 2", len(page1))
	}
	if nextToken == "" {
		t.Error("expected non-empty nextToken after page1")
	}

	// Page 2: next 2.
	page2, nextToken2, err := s.ListChannels(ctx, testParent, 2, nextToken)
	if err != nil {
		t.Fatalf("ListChannels page2: %v", err)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
	if nextToken2 == "" {
		t.Error("expected non-empty nextToken after page2")
	}

	// Page 3: last 1.
	page3, nextToken3, err := s.ListChannels(ctx, testParent, 2, nextToken2)
	if err != nil {
		t.Fatalf("ListChannels page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3 len = %d, want 1", len(page3))
	}
	if nextToken3 != "" {
		t.Errorf("expected empty nextToken at end, got %q", nextToken3)
	}

	// Verify no duplicates across pages.
	seen := make(map[string]bool)
	for _, pg := range [][]*eventarcpb.Channel{page1, page2, page3} {
		for _, ch := range pg {
			if seen[ch.GetName()] {
				t.Errorf("duplicate channel %q in paginated results", ch.GetName())
			}
			seen[ch.GetName()] = true
		}
	}
	if len(seen) != 5 {
		t.Errorf("total unique channels across pages = %d, want 5", len(seen))
	}
}

// TestStorageCreateChannelConnection_Success verifies that a new channel
// connection is stored and returned with server-assigned fields.
func TestStorageCreateChannelConnection_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	conn := &eventarcpb.ChannelConnection{}
	got, err := s.CreateChannelConnection(ctx, testParent, "my-conn", conn)
	if err != nil {
		t.Fatalf("CreateChannelConnection: unexpected error: %v", err)
	}
	wantName := testParent + "/channelConnections/my-conn"
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
}

// TestStorageGetChannelConnection_NotFound verifies that looking up a
// non-existent channel connection returns a NotFound error.
func TestStorageGetChannelConnection_NotFound(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	_, err := s.GetChannelConnection(ctx, testParent+"/channelConnections/does-not-exist")
	if err == nil {
		t.Fatal("expected NotFound error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code = %v, want NotFound", err)
	}
}

// TestStorageDeleteChannelConnection_Success verifies that creating then
// deleting a channel connection leaves it absent.
func TestStorageDeleteChannelConnection_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	conn := &eventarcpb.ChannelConnection{}
	created, err := s.CreateChannelConnection(ctx, testParent, "to-delete", conn)
	if err != nil {
		t.Fatalf("CreateChannelConnection: unexpected error: %v", err)
	}

	if err := s.DeleteChannelConnection(ctx, created.GetName()); err != nil {
		t.Fatalf("DeleteChannelConnection: unexpected error: %v", err)
	}

	// Verify it's gone.
	_, err = s.GetChannelConnection(ctx, created.GetName())
	if err == nil {
		t.Fatal("expected NotFound after delete, got nil error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("error code after delete = %v, want NotFound", err)
	}
}

// TestStorageGetGoogleChannelConfig_Default verifies that getting a config
// that has never been set returns a zero-value config (not an error).
func TestStorageGetGoogleChannelConfig_Default(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	name := testParent + "/googleChannelConfig"
	got, err := s.GetGoogleChannelConfig(ctx, name)
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig: unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("GetGoogleChannelConfig: expected non-nil result for missing config")
	}
	if got.GetName() != name {
		t.Errorf("Name = %q, want %q", got.GetName(), name)
	}
}

// TestCreateChannel_StateIsActive was updated: initial state is now PENDING.
// See TestCreateChannel_InitialStatePending.

// TestCreateChannel_InitialStatePending verifies that a newly created channel
// has its State field set to PENDING (not ACTIVE).
func TestCreateChannel_InitialStatePending(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	ch := &eventarcpb.Channel{}
	got, err := s.CreateChannel(ctx, testParent, "state-test-channel", ch)
	if err != nil {
		t.Fatalf("CreateChannel: unexpected error: %v", err)
	}
	if got.GetState() != eventarcpb.Channel_PENDING {
		t.Errorf("State = %v, want PENDING", got.GetState())
	}
}

// TestCreateChannel_ActivationTokenSet verifies that a newly created channel
// has a non-empty ActivationToken.
func TestCreateChannel_ActivationTokenSet(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	ch := &eventarcpb.Channel{}
	got, err := s.CreateChannel(ctx, testParent, "token-test-channel", ch)
	if err != nil {
		t.Fatalf("CreateChannel: unexpected error: %v", err)
	}
	if got.GetActivationToken() == "" {
		t.Error("ActivationToken should not be empty after create")
	}
}

// TestCreateChannelConnection_ActivationTokenCleared verifies that an
// activation_token supplied in the input is not persisted on the stored
// ChannelConnection.
func TestCreateChannelConnection_ActivationTokenCleared(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	conn := &eventarcpb.ChannelConnection{
		ActivationToken: "should-be-cleared",
	}
	got, err := s.CreateChannelConnection(ctx, testParent, "token-conn", conn)
	if err != nil {
		t.Fatalf("CreateChannelConnection: unexpected error: %v", err)
	}
	if got.GetActivationToken() != "" {
		t.Errorf("ActivationToken = %q, want empty string", got.GetActivationToken())
	}
}

// TestGetGoogleChannelConfig_StableUpdateTime verifies that calling
// GetGoogleChannelConfig twice for the same name returns the same UpdateTime
// (i.e. the default is initialized once, not regenerated on each call).
func TestGetGoogleChannelConfig_StableUpdateTime(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	name := testParent + "/googleChannelConfig"
	first, err := s.GetGoogleChannelConfig(ctx, name)
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig (1st): %v", err)
	}
	second, err := s.GetGoogleChannelConfig(ctx, name)
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig (2nd): %v", err)
	}
	t1 := first.GetUpdateTime().AsTime()
	t2 := second.GetUpdateTime().AsTime()
	if !t1.Equal(t2) {
		t.Errorf("UpdateTime changed between calls: %v != %v", t1, t2)
	}
}

// TestUpdateGoogleChannelConfig_NameRequired verifies that calling
// UpdateGoogleChannelConfig with an empty name returns InvalidArgument.
func TestUpdateGoogleChannelConfig_NameRequired(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	cfg := &eventarcpb.GoogleChannelConfig{
		// Name intentionally empty.
		CryptoKeyName: "projects/p/locations/l/keyRings/kr/cryptoKeys/k",
	}
	_, err := s.UpdateGoogleChannelConfig(ctx, cfg, nil)
	if err == nil {
		t.Fatal("expected InvalidArgument error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("error code = %v, want InvalidArgument", err)
	}
}

// TestGetGoogleChannelConfig_DefaultHasUpdateTime verifies that getting a config
// that has never been stored returns a non-nil UpdateTime.
func TestGetGoogleChannelConfig_DefaultHasUpdateTime(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	name := testParent + "/googleChannelConfig"
	got, err := s.GetGoogleChannelConfig(ctx, name)
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig: unexpected error: %v", err)
	}
	if got.GetUpdateTime() == nil {
		t.Error("UpdateTime should not be nil for default config")
	}
}

// TestGetChannelExists verifies that GetChannelExists returns true for a stored
// channel and false for a channel that does not exist.
func TestGetChannelExists(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	// Channel that does not exist yet.
	name := testParent + "/channels/existence-test"
	exists, err := s.GetChannelExists(ctx, name)
	if err != nil {
		t.Fatalf("GetChannelExists (missing): unexpected error: %v", err)
	}
	if exists {
		t.Error("GetChannelExists = true for non-existent channel, want false")
	}

	// Create the channel.
	if _, err := s.CreateChannel(ctx, testParent, "existence-test", &eventarcpb.Channel{}); err != nil {
		t.Fatalf("CreateChannel: unexpected error: %v", err)
	}

	// Channel should now exist.
	exists, err = s.GetChannelExists(ctx, name)
	if err != nil {
		t.Fatalf("GetChannelExists (present): unexpected error: %v", err)
	}
	if !exists {
		t.Error("GetChannelExists = false for existing channel, want true")
	}
}

// TestStorageUpdateGoogleChannelConfig_Success verifies that updating a config
// sets UpdateTime and applies specified fields.
func TestStorageUpdateGoogleChannelConfig_Success(t *testing.T) {
	s := newTestStorage(t)
	ctx := context.Background()

	name := testParent + "/googleChannelConfig"
	cfg := &eventarcpb.GoogleChannelConfig{
		Name:          name,
		CryptoKeyName: "projects/p/locations/l/keyRings/kr/cryptoKeys/k",
	}

	got, err := s.UpdateGoogleChannelConfig(ctx, cfg, nil)
	if err != nil {
		t.Fatalf("UpdateGoogleChannelConfig: unexpected error: %v", err)
	}
	if got.GetCryptoKeyName() != cfg.GetCryptoKeyName() {
		t.Errorf("CryptoKeyName = %q, want %q", got.GetCryptoKeyName(), cfg.GetCryptoKeyName())
	}
	if got.GetUpdateTime() == nil {
		t.Error("UpdateTime should not be nil after update")
	}

	// Verify the stored config is retrievable.
	stored, err := s.GetGoogleChannelConfig(ctx, name)
	if err != nil {
		t.Fatalf("GetGoogleChannelConfig after update: %v", err)
	}
	if stored.GetCryptoKeyName() != cfg.GetCryptoKeyName() {
		t.Errorf("stored CryptoKeyName = %q, want %q", stored.GetCryptoKeyName(), cfg.GetCryptoKeyName())
	}
}
