package authz

import "testing"

func TestGetPermission_KnownOperation(t *testing.T) {
	tests := []struct {
		name           string
		operation      string
		wantPermission string
	}{
		{
			name:           "GetTrigger",
			operation:      "GetTrigger",
			wantPermission: "eventarc.triggers.get",
		},
		{
			name:           "ListTriggers",
			operation:      "ListTriggers",
			wantPermission: "eventarc.triggers.list",
		},
		{
			name:           "CreateTrigger",
			operation:      "CreateTrigger",
			wantPermission: "eventarc.triggers.create",
		},
		{
			name:           "UpdateTrigger",
			operation:      "UpdateTrigger",
			wantPermission: "eventarc.triggers.update",
		},
		{
			name:           "DeleteTrigger",
			operation:      "DeleteTrigger",
			wantPermission: "eventarc.triggers.delete",
		},
		{
			name:           "GetProvider",
			operation:      "GetProvider",
			wantPermission: "eventarc.providers.get",
		},
		{
			name:           "ListProviders",
			operation:      "ListProviders",
			wantPermission: "eventarc.providers.list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			perm, ok := GetPermission(tt.operation)
			if !ok {
				t.Fatalf("GetPermission(%q) returned ok=false, want true", tt.operation)
			}
			if perm.Permission != tt.wantPermission {
				t.Errorf("GetPermission(%q).Permission = %q, want %q", tt.operation, perm.Permission, tt.wantPermission)
			}
		})
	}
}

func TestGetPermission_UnknownOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation string
	}{
		{name: "UnknownOperation", operation: "UnknownOperation"},
		{name: "EmptyOperation", operation: ""},
		{name: "CaseSensitive_lowercase", operation: "gettrigger"},
		{name: "CaseSensitive_uppercase", operation: "GETTRIGGER"},
		{name: "NonExistentRPC", operation: "PublishEvents"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := GetPermission(tt.operation)
			if ok {
				t.Errorf("GetPermission(%q) returned ok=true, want false", tt.operation)
			}
		})
	}
}

func TestNormalizeTriggerResource(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "FullTriggerName",
			input: "projects/my-project/locations/us-central1/triggers/my-trigger",
			want:  "projects/my-project/locations/us-central1/triggers/my-trigger",
		},
		{
			name:  "EmptyString",
			input: "",
			want:  "",
		},
		{
			name:  "PartialName",
			input: "projects/my-project/locations/us-central1/triggers/",
			want:  "projects/my-project/locations/us-central1/triggers/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTriggerResource(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeTriggerResource(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeParentForCreate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "NormalParent",
			input: "projects/my-project/locations/us-central1",
			want:  "projects/my-project/locations/us-central1",
		},
		{
			name:  "TrailingSlash",
			input: "projects/my-project/locations/us-central1/",
			want:  "projects/my-project/locations/us-central1",
		},
		{
			name:  "MultipleTrailingSlashes",
			input: "projects/my-project/locations/us-central1///",
			want:  "projects/my-project/locations/us-central1",
		},
		{
			name:  "EmptyString",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeParentForCreate(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeParentForCreate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
