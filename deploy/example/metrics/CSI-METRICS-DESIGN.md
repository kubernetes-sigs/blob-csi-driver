# CSI Metrics Design

## Overview

The blob CSI driver now emits two distinct types of metrics:

1. **CSI Operation Metrics** (`blob_csi_driver_*`) - For CSI interface operations
2. **Cloud Provider Metrics** (`cloudprovider_azure_op_*`) - For Azure API calls only

## CSI Operation Metrics

These metrics track CSI operations (CreateVolume, NodeStageVolume, NodePublishVolume, etc.) independently from Azure cloud provider operations.

### Metrics Exposed

#### `blob_csi_driver_operation_duration_seconds`
Histogram of CSI operation duration in seconds.

**Labels:**
- `operation`: CSI operation name (e.g., `controller_create_volume`, `node_stage_volume`)
- `success`: Operation success status (`true`/`false`)

**Example:**
```promql
blob_csi_driver_operation_duration_seconds_bucket{operation="node_stage_volume",success="true"}
```

#### `blob_csi_driver_operation_duration_seconds_labeled`
Histogram of CSI operation duration with additional context labels.

**Labels:**
- `operation`: CSI operation name
- `success`: Operation success status
- `protocol`: Mount protocol (nfs, fuse, fuse2)
- `storage_account_type`: Azure storage account SKU
- `is_hns_enabled`: Whether hierarchical namespace is enabled

**Example:**
```promql
blob_csi_driver_operation_duration_seconds_labeled{
  operation="node_stage_volume",
  success="true",
  protocol="fuse2",
  storage_account_type="Premium_LRS",
  is_hns_enabled="false"
}
```

#### `blob_csi_driver_operations_total`
Counter of total CSI operations.

**Labels:**
- `operation`: CSI operation name
- `success`: Operation success status

**Example:**
```promql
rate(blob_csi_driver_operations_total{operation="node_publish_volume"}[5m])
```

## Cloud Provider Metrics

These metrics continue to use the existing `cloudprovider_azure_op_duration_seconds` metric for actual Azure API calls (storage account creation, container management, etc.).

**When used:**
- Storage account creation/deletion
- Blob container creation/deletion
- Azure resource management operations

## Operation Mapping

### Controller Operations
| CSI Operation | Metric Name | Azure API Calls |
|--------------|-------------|-----------------|
| CreateVolume | `controller_create_volume` | EnsureStorageAccount, CreateContainer (tracked separately in cloud provider metrics) |
| DeleteVolume | `controller_delete_volume` | DeleteContainer (tracked separately) |
| ValidateVolumeCapabilities | `controller_validate_volume_capabilities` | None |
| ControllerExpandVolume | `controller_expand_volume` | None |

### Node Operations  
| CSI Operation | Metric Name | Azure API Calls |
|--------------|-------------|-----------------|
| NodeStageVolume | `node_stage_volume` | None (mount operation) |
| NodeUnstageVolume | `node_unstage_volume` | None (unmount operation) |
| NodePublishVolume | `node_publish_volume` | None (bind mount) |
| NodeUnpublishVolume | `node_unpublish_volume` | None (unmount) |
| NodeGetInfo | `node_get_info` | None |
| NodeGetVolumeStats | `node_get_volume_stats` | None |

## Example Queries

### Average mount duration by protocol
```promql
avg(blob_csi_driver_operation_duration_seconds_labeled{
  operation="node_stage_volume",
  success="true"
}) by (protocol)
```

### Mount failure rate
```promql
rate(blob_csi_driver_operations_total{
  operation="node_stage_volume",
  success="false"
}[5m])
```

### Volume creation latency p95
```promql
histogram_quantile(0.95,
  rate(blob_csi_driver_operation_duration_seconds_bucket{
    operation="controller_create_volume"
  }[5m])
)
```

### Operations by storage account type
```promql
sum(rate(blob_csi_driver_operations_total[5m])) 
by (storage_account_type, operation)
```
