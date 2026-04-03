package server

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// resourceIDRe matches valid GCP resource IDs: lowercase letter, then up to
// 61 alphanumeric-or-hyphen chars, ending in alphanumeric.
var resourceIDRe = regexp.MustCompile(`^[a-z]([a-z0-9\-]{0,61}[a-z0-9])?$`)

// validateResourceID returns InvalidArgument if id does not match the
// standard GCP resource ID format.
func validateResourceID(id, field string) error {
	if !resourceIDRe.MatchString(id) {
		return status.Errorf(codes.InvalidArgument,
			"%s must match ^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$, got %q", field, id)
	}
	return nil
}

// PaginatePage slices items into a page using integer-offset tokens.
// pageToken is an integer string offset; empty string means start from 0.
// pageSize is clamped to [1, 100]; 0 or negative defaults to 100.
// Returns (page, nextToken, err). nextToken is "" when this is the last page.
func PaginatePage[T any](items []T, pageToken string, pageSize int32) ([]T, string, error) {
	offset := 0
	if pageToken != "" {
		n, err := strconv.Atoi(pageToken)
		if err != nil || n < 0 {
			return nil, "", status.Errorf(codes.InvalidArgument, "invalid page_token")
		}
		offset = n
	}
	size := int(pageSize)
	if size <= 0 || size > 100 {
		size = 100
	}
	end := offset + size
	if end > len(items) {
		end = len(items)
	}
	nextToken := ""
	if end < len(items) {
		nextToken = strconv.Itoa(end)
	}
	if offset >= len(items) {
		return nil, nextToken, nil
	}
	return items[offset:end], nextToken, nil
}

// cloneProto returns a deep copy of any proto.Message using proto.Clone.
// Returns the zero value of T if m is nil (detected via reflection).
func cloneProto[T proto.Message](m T) T {
	if reflect.ValueOf(m).IsNil() {
		var zero T
		return zero
	}
	return proto.Clone(m).(T)
}

// newUID returns a random 64-bit hex string for use as a resource UID.
func newUID() string { return fmt.Sprintf("%x", rand.Uint64()) }

// newEtag returns a random 64-bit hex string for use as a resource etag.
func newEtag() string { return fmt.Sprintf("%x", rand.Uint64()) }
