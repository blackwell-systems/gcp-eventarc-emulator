package authz

import "strings"

// PermissionCheck maps an RPC name to its GCP IAM permission string.
type PermissionCheck struct {
	Permission string
}

// permissionMap maps Eventarc RPC operation names to their required IAM permissions.
var permissionMap = map[string]PermissionCheck{
	"GetTrigger":    {Permission: "eventarc.triggers.get"},
	"ListTriggers":  {Permission: "eventarc.triggers.list"},
	"CreateTrigger": {Permission: "eventarc.triggers.create"},
	"UpdateTrigger": {Permission: "eventarc.triggers.update"},
	"DeleteTrigger": {Permission: "eventarc.triggers.delete"},
	"GetProvider":   {Permission: "eventarc.providers.get"},
	"ListProviders": {Permission: "eventarc.providers.list"},
}

// GetPermission returns the PermissionCheck for the named operation.
// Returns false if the operation is not mapped.
func GetPermission(operation string) (PermissionCheck, bool) {
	perm, ok := permissionMap[operation]
	return perm, ok
}

// NormalizeParentForCreate strips any trailing slash from parent and returns it as-is.
// Used before IAM checks on create operations to canonicalize the parent resource name.
func NormalizeParentForCreate(parent string) string {
	return strings.TrimRight(parent, "/")
}

// NormalizeTriggerResource returns the trigger resource name for IAM checks.
// The name is returned as-is; this function is a hook for any future normalization.
func NormalizeTriggerResource(name string) string {
	return name
}
