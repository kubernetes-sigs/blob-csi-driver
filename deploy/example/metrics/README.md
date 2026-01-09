# Prometheus Metrics for CSI Blob Driver

This directory contains manifests and tools for monitoring the Azure Blob CSI driver with Prometheus.

## Metrics Endpoints

The CSI driver exposes Prometheus metrics on two endpoints:

- **Controller**: Port `29634` - Metrics for volume provisioning/deletion operations
- **Node**: Port `29635` - Metrics for mount/unmount operations on each node

## Quick Start

### Option 1: Deploy Full Prometheus Stack (Recommended)

Deploy Prometheus with auto-discovery of CSI driver metrics:

```bash
# Deploy Prometheus
kubectl apply -f deploy/example/metrics/prometheus-setup.yaml

# Wait for Prometheus to be ready
kubectl wait --for=condition=ready pod -l app=prometheus -n monitoring --timeout=120s

# Access Prometheus UI
kubectl port-forward -n monitoring svc/prometheus 9090:9090
# Then open http://localhost:9090 in your browser
```

### Option 2: Manual Metrics Access

Create services to access metrics directly:

```bash
# Create controller service
kubectl apply -f deploy/example/metrics/csi-blob-controller-svc.yaml

# Get controller metrics
kubectl get svc csi-blob-controller -n kube-system
ip=$(kubectl get svc csi-blob-controller -n kube-system -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
curl http://$ip:29634/metrics
```

## Available Metrics

### Azure Cloud Provider Metrics
- `cloudprovider_azure_api_request_duration_seconds` - API request latency
- `cloudprovider_azure_api_request_errors` - API request errors
- `cloudprovider_azure_api_request_throttled_count` - Throttled requests

### CSI Operation Metrics

**Controller Operations:**
- `controller_create_volume` - Volume creation operations
- `controller_delete_volume` - Volume deletion operations
- `controller_validate_volume_capabilities` - Volume validation

**Node Operations:**
- `node_stage_volume` - Volume staging (mounting)
- `node_unstage_volume` - Volume unstaging (unmounting)
- `node_publish_volume` - Volume publishing
- `node_unpublish_volume` - Volume unpublishing
- `node_get_volume_stats` - Volume statistics retrieval

## Example PromQL Queries

### Operation Success Rate
```promql
# Controller create volume success rate
rate(blob_csi_driver_operations_total{operation="controller_create_volume",success="true"}[5m]) / 
rate(blob_csi_driver_operations_total{operation="controller_create_volume"}[5m])

# Node stage volume success rate  
rate(blob_csi_driver_operations_total{operation="node_stage_volume",success="true"}[5m]) / 
rate(blob_csi_driver_operations_total{operation="node_stage_volume"}[5m])
```

### Operations by Protocol
```promql
# NFS mount operations
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{operation="node_stage_volume",protocol="nfs"}[5m]))

# Blobfuse mount operations  
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{operation="node_stage_volume",protocol="fuse"}[5m]))

# Blobfuse2 mount operations
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{operation="node_stage_volume",protocol="fuse2"}[5m]))
```

### Operations by Storage Account Type
```promql
# Premium storage operations
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{storage_account_type="Premium_LRS"}[5m]))

# Standard storage operations
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{storage_account_type="Standard_LRS"}[5m]))
```

### Error Rate
```promql
# Failed operations by type
sum(rate(blob_csi_driver_operations_total{success="false"}[5m])) by (operation)

# Failed mount operations
rate(blob_csi_driver_operations_total{operation="node_stage_volume",success="false"}[5m])
```

### Latency Percentiles
```promql
# 95th percentile latency for volume creation
histogram_quantile(0.95, 
  rate(blob_csi_driver_operation_duration_seconds_bucket{operation="controller_create_volume"}[5m])
)

# 95th percentile latency for volume mounting with labels
histogram_quantile(0.95,
  rate(blob_csi_driver_operation_duration_seconds_labeled_bucket{operation="node_stage_volume"}[5m])
)
```

### HNS-enabled Operations
```promql
# Operations with HNS enabled
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{is_hns_enabled="true"}[5m]))

# Operations with HNS disabled  
sum(rate(blob_csi_driver_operation_duration_seconds_labeled_count{is_hns_enabled="false"}[5m]))
```

## Grafana Dashboards

After deploying Prometheus, you can import Grafana dashboards to visualize metrics:

1. Deploy Grafana (optional):
```bash
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/kube-prometheus/main/manifests/grafana-deployment.yaml
```

2. Configure Prometheus as a data source
3. Create custom dashboards using the PromQL queries above

## Troubleshooting

### Metrics not appearing

1. Check pods are running:
```bash
kubectl get pods -n kube-system -l app=csi-blob-controller
kubectl get pods -n kube-system -l app=csi-blob-node
```

2. Check metrics endpoint directly:
```bash
# Controller
kubectl exec -n kube-system <controller-pod> -- curl http://localhost:29634/metrics

# Node
kubectl exec -n kube-system <node-pod> -c blob -- curl http://localhost:29635/metrics
```

3. Check Prometheus targets:
- Open Prometheus UI at http://localhost:9090
- Go to Status → Targets
- Verify `csi-blob-controller` and `csi-blob-node` targets are UP

### No dimensions in metrics

Dimensions (protocol, storageAccountType, etc.) only appear after operations are performed. Create test volumes to generate metrics with full dimensions:

```bash
# Create a test volume
kubectl create -f deploy/example/storageclass-blobfuse.yaml
kubectl create -f deploy/example/pvc-blobfuse.yaml
```

## Additional Resources

- [Prometheus Documentation](https://prometheus.io/docs/)
- [PromQL Basics](https://prometheus.io/docs/prometheus/latest/querying/basics/)

