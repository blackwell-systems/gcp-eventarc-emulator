# API Coverage

Implements the full Eventarc v1 API surface, compatible with the official GCP Eventarc clients.

## Eventarc Service (40 RPCs)

| Resource | RPCs |
|----------|------|
| Triggers | CreateTrigger, GetTrigger, UpdateTrigger, DeleteTrigger, ListTriggers |
| Channels | CreateChannel, GetChannel, UpdateChannel, DeleteChannel, ListChannels |
| Channel Connections | CreateChannelConnection, GetChannelConnection, DeleteChannelConnection, ListChannelConnections |
| Google Channel Config | GetGoogleChannelConfig, UpdateGoogleChannelConfig |
| Message Buses | CreateMessageBus, GetMessageBus, UpdateMessageBus, DeleteMessageBus, ListMessageBuses, ListMessageBusEnrollments |
| Enrollments | CreateEnrollment, GetEnrollment, UpdateEnrollment, DeleteEnrollment, ListEnrollments |
| Pipelines | CreatePipeline, GetPipeline, UpdatePipeline, DeletePipeline, ListPipelines |
| Google API Sources | CreateGoogleApiSource, GetGoogleApiSource, UpdateGoogleApiSource, DeleteGoogleApiSource, ListGoogleApiSources |
| Providers | GetProvider, ListProviders |

## Publishing Service (2 RPCs)

- `PublishEvents`
- `PublishChannelConnectionEvents`

## Operations Service (5 RPCs)

All Create/Update/Delete operations return `google.longrunning.Operation` (resolved immediately).

- `GetOperation`, `ListOperations`, `DeleteOperation`, `CancelOperation`, `WaitOperation`

**Total: 47 RPCs**

---

## IAM Permissions

| Operation | Permission |
|-----------|-----------|
| CreateTrigger | `eventarc.triggers.create` |
| GetTrigger | `eventarc.triggers.get` |
| UpdateTrigger | `eventarc.triggers.update` |
| DeleteTrigger | `eventarc.triggers.delete` |
| ListTriggers | `eventarc.triggers.list` |
| CreateChannel | `eventarc.channels.create` |
| GetChannel | `eventarc.channels.get` |
| UpdateChannel | `eventarc.channels.update` |
| DeleteChannel | `eventarc.channels.delete` |
| ListChannels | `eventarc.channels.list` |
| CreateChannelConnection | `eventarc.channelConnections.create` |
| GetChannelConnection | `eventarc.channelConnections.get` |
| DeleteChannelConnection | `eventarc.channelConnections.delete` |
| ListChannelConnections | `eventarc.channelConnections.list` |
| GetGoogleChannelConfig | `eventarc.googleChannelConfigs.get` |
| UpdateGoogleChannelConfig | `eventarc.googleChannelConfigs.update` |
| GetProvider | `eventarc.providers.get` |
| ListProviders | `eventarc.providers.list` |
| CreateMessageBus | `eventarc.messageBuses.create` |
| GetMessageBus | `eventarc.messageBuses.get` |
| UpdateMessageBus | `eventarc.messageBuses.update` |
| DeleteMessageBus | `eventarc.messageBuses.delete` |
| ListMessageBuses | `eventarc.messageBuses.list` |
| ListMessageBusEnrollments | `eventarc.messageBuses.list` |
| CreateEnrollment | `eventarc.enrollments.create` |
| GetEnrollment | `eventarc.enrollments.get` |
| UpdateEnrollment | `eventarc.enrollments.update` |
| DeleteEnrollment | `eventarc.enrollments.delete` |
| ListEnrollments | `eventarc.enrollments.list` |
| CreatePipeline | `eventarc.pipelines.create` |
| GetPipeline | `eventarc.pipelines.get` |
| UpdatePipeline | `eventarc.pipelines.update` |
| DeletePipeline | `eventarc.pipelines.delete` |
| ListPipelines | `eventarc.pipelines.list` |
| CreateGoogleApiSource | `eventarc.googleApiSources.create` |
| GetGoogleApiSource | `eventarc.googleApiSources.get` |
| UpdateGoogleApiSource | `eventarc.googleApiSources.update` |
| DeleteGoogleApiSource | `eventarc.googleApiSources.delete` |
| ListGoogleApiSources | `eventarc.googleApiSources.list` |
