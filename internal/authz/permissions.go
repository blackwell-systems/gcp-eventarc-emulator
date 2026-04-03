// Package authz maps Eventarc RPC methods to their GCP IAM permission strings.
//
// GetPermission returns the PermissionCheck for a given RPC name (e.g.
// "GetTrigger" → "eventarc.triggers.get"). All 39 Eventarc IAM permissions
// are registered. NormalizeParentForCreate strips trailing slashes from
// parent resource names before IAM checks.
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
	// Channel
	"GetChannel":    {Permission: "eventarc.channels.get"},
	"ListChannels":  {Permission: "eventarc.channels.list"},
	"CreateChannel": {Permission: "eventarc.channels.create"},
	"UpdateChannel": {Permission: "eventarc.channels.update"},
	"DeleteChannel": {Permission: "eventarc.channels.delete"},
	// ChannelConnection
	"GetChannelConnection":    {Permission: "eventarc.channelConnections.get"},
	"ListChannelConnections":  {Permission: "eventarc.channelConnections.list"},
	"CreateChannelConnection": {Permission: "eventarc.channelConnections.create"},
	"DeleteChannelConnection": {Permission: "eventarc.channelConnections.delete"},
	// GoogleChannelConfig
	"GetGoogleChannelConfig":    {Permission: "eventarc.googleChannelConfigs.get"},
	"UpdateGoogleChannelConfig": {Permission: "eventarc.googleChannelConfigs.update"},
	// MessageBus
	"GetMessageBus":             {Permission: "eventarc.messageBuses.get"},
	"ListMessageBuses":          {Permission: "eventarc.messageBuses.list"},
	"ListMessageBusEnrollments": {Permission: "eventarc.messageBuses.listEnrollments"},
	"CreateMessageBus":          {Permission: "eventarc.messageBuses.create"},
	"UpdateMessageBus":          {Permission: "eventarc.messageBuses.update"},
	"DeleteMessageBus":          {Permission: "eventarc.messageBuses.delete"},
	// Enrollment
	"GetEnrollment":    {Permission: "eventarc.enrollments.get"},
	"ListEnrollments":  {Permission: "eventarc.enrollments.list"},
	"CreateEnrollment": {Permission: "eventarc.enrollments.create"},
	"UpdateEnrollment": {Permission: "eventarc.enrollments.update"},
	"DeleteEnrollment": {Permission: "eventarc.enrollments.delete"},
	// Pipeline
	"GetPipeline":    {Permission: "eventarc.pipelines.get"},
	"ListPipelines":  {Permission: "eventarc.pipelines.list"},
	"CreatePipeline": {Permission: "eventarc.pipelines.create"},
	"UpdatePipeline": {Permission: "eventarc.pipelines.update"},
	"DeletePipeline": {Permission: "eventarc.pipelines.delete"},
	// GoogleApiSource
	"GetGoogleApiSource":    {Permission: "eventarc.googleApiSources.get"},
	"ListGoogleApiSources":  {Permission: "eventarc.googleApiSources.list"},
	"CreateGoogleApiSource": {Permission: "eventarc.googleApiSources.create"},
	"UpdateGoogleApiSource": {Permission: "eventarc.googleApiSources.update"},
	"DeleteGoogleApiSource": {Permission: "eventarc.googleApiSources.delete"},
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
