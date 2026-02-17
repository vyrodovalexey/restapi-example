# restapi-example Helm Chart

A comprehensive Helm chart for deploying the REST API and WebSocket Server on Kubernetes with production-ready features including multiple authentication modes, TLS/mTLS support, autoscaling, monitoring, and Vault integration.

## Features

- **Multiple Authentication Modes** - Support for none, mTLS, OIDC, Basic Auth, API Key, and Multi-mode
- **TLS/mTLS Configuration** - Secure communication with client certificate authentication
- **Vault Integration** - Dynamic PKI certificate management
- **Horizontal Pod Autoscaler** - Automatic scaling based on CPU/memory metrics
- **Pod Disruption Budget** - High availability during cluster maintenance
- **ServiceMonitor** - Prometheus metrics scraping integration
- **Ingress Support** - HTTP/HTTPS routing with TLS termination
- **Security Best Practices** - Non-root containers, security contexts, network policies
- **ConfigMap & Secret Management** - Secure configuration and credential handling

## Prerequisites

- Kubernetes 1.23+
- Helm 3.0+
- Prometheus Operator (optional, for ServiceMonitor)
- Cert-Manager (optional, for automatic TLS certificates)
- Vault (optional, for dynamic certificate management)

## Installation

### Quick Start

```bash
# Basic installation
helm install my-api ./helm/restapi-example

# With custom values
helm install my-api ./helm/restapi-example -f my-values.yaml

# From specific namespace
helm install my-api ./helm/restapi-example -n production --create-namespace
```

### Installation Examples

#### Development Environment
```bash
helm install dev-api ./helm/restapi-example \
  --set replicaCount=1 \
  --set config.logLevel=debug \
  --set config.auth.mode=none
```

#### Production Environment
```bash
helm install prod-api ./helm/restapi-example \
  --set replicaCount=3 \
  --set autoscaling.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set serviceMonitor.enabled=true \
  --set config.auth.mode=apikey \
  --set config.apiKey.keys="prod-key-001:frontend,prod-key-002:mobile"
```

## Configuration

See [values.yaml](values.yaml) for the complete list of configurable parameters.

### Core Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `ghcr.io/vyrodovalexey/restapi-example` |
| `image.tag` | Image tag (defaults to chart appVersion) | `""` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |

### Service Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `service.type` | Service type | `ClusterIP` |
| `service.port` | HTTP service port | `8080` |
| `service.targetPort` | Container target port | `8080` |

### Application Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.logLevel` | Log level (debug, info, warn, error) | `info` |
| `config.metricsEnabled` | Enable Prometheus metrics | `true` |
| `config.shutdownTimeout` | Graceful shutdown timeout | `30s` |

### Authentication Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.auth.mode` | Auth mode (none, mtls, oidc, basic, apikey, multi) | `none` |
| `config.apiKey.keys` | API keys (key:name,key:name,...) | `""` |
| `config.basicAuth.users` | Basic auth users (user:hash,user:hash,...) | `""` |
| `config.oidc.issuerURL` | OIDC issuer URL | `""` |
| `config.oidc.clientID` | OIDC client ID | `""` |
| `config.oidc.audience` | OIDC audience | `""` |

### TLS Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.tls.enabled` | Enable TLS | `false` |
| `config.tls.existingSecret` | Existing TLS secret name | `""` |
| `config.tls.clientAuth` | TLS client auth (none, request, require) | `none` |

### Vault Integration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `vault.enabled` | Enable Vault integration | `false` |
| `vault.address` | Vault server address | `""` |
| `vault.token` | Vault authentication token | `""` |
| `vault.pkiPath` | Vault PKI path | `""` |
| `vault.pkiRole` | Vault PKI role | `""` |

### Autoscaling Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `autoscaling.enabled` | Enable HPA | `false` |
| `autoscaling.minReplicas` | Minimum replicas | `1` |
| `autoscaling.maxReplicas` | Maximum replicas | `100` |
| `autoscaling.targetCPUUtilizationPercentage` | Target CPU utilization | `80` |
| `autoscaling.targetMemoryUtilizationPercentage` | Target memory utilization | `""` |

### High Availability Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podDisruptionBudget.enabled` | Enable PDB | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods | `""` |
| `podDisruptionBudget.maxUnavailable` | Maximum unavailable pods | `""` |

### Monitoring Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceMonitor.enabled` | Enable ServiceMonitor for Prometheus | `false` |
| `serviceMonitor.interval` | Scrape interval | `30s` |
| `serviceMonitor.path` | Metrics path | `/metrics` |
| `serviceMonitor.labels` | Additional labels | `{}` |

### Ingress Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ingress.enabled` | Enable Ingress | `false` |
| `ingress.className` | Ingress class name | `""` |
| `ingress.annotations` | Ingress annotations | `{}` |
| `ingress.hosts` | Ingress hosts configuration | `[]` |
| `ingress.tls` | Ingress TLS configuration | `[]` |

## Configuration Examples

### API Key Authentication

```yaml
# values-apikey.yaml
config:
  auth:
    mode: apikey
  apiKey:
    keys: "frontend-key:frontend-app,mobile-key:mobile-app,admin-key:admin-dashboard"

# Deploy
helm install my-api ./helm/restapi-example -f values-apikey.yaml
```

### Basic Authentication

```yaml
# values-basic.yaml
config:
  auth:
    mode: basic
  basicAuth:
    # Generate with: htpasswd -nbBC 10 "" password | tr -d ':\n'
    users: "admin:$2y$10$...,user:$2y$10$..."

# Deploy
helm install my-api ./helm/restapi-example -f values-basic.yaml
```

### OIDC Authentication with Keycloak

```yaml
# values-oidc.yaml
config:
  auth:
    mode: oidc
  oidc:
    issuerURL: "https://keycloak.example.com/realms/production"
    clientID: "restapi-server"
    audience: "restapi-audience"

# Deploy
helm install my-api ./helm/restapi-example -f values-oidc.yaml
```

### TLS with mTLS Authentication

```yaml
# values-mtls.yaml
config:
  tls:
    enabled: true
    existingSecret: "restapi-tls-secret"
    clientAuth: require
  auth:
    mode: mtls

# Create TLS secret first
kubectl create secret tls restapi-tls-secret \
  --cert=server.crt \
  --key=server.key

# Deploy
helm install my-api ./helm/restapi-example -f values-mtls.yaml
```

### Multi-Mode Authentication

```yaml
# values-multi.yaml
config:
  auth:
    mode: multi
  apiKey:
    keys: "api-key-001:service-a"
  basicAuth:
    users: "admin:$2y$10$..."
  tls:
    enabled: true
    existingSecret: "restapi-tls-secret"

# Deploy
helm install my-api ./helm/restapi-example -f values-multi.yaml
```

### Production Deployment with Full Features

```yaml
# values-production.yaml
replicaCount: 3

image:
  repository: ghcr.io/vyrodovalexey/restapi-example
  tag: "v1.0.0"
  pullPolicy: IfNotPresent

config:
  logLevel: info
  metricsEnabled: true
  auth:
    mode: apikey
  apiKey:
    keys: "prod-frontend:frontend,prod-mobile:mobile,prod-admin:admin"

service:
  type: ClusterIP
  port: 8080

ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  hosts:
    - host: api.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: api-example-com-tls
      hosts:
        - api.example.com

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

podDisruptionBudget:
  enabled: true
  minAvailable: 2

serviceMonitor:
  enabled: true
  interval: 30s
  labels:
    release: prometheus

resources:
  limits:
    cpu: 1000m
    memory: 512Mi
  requests:
    cpu: 200m
    memory: 256Mi

nodeSelector:
  kubernetes.io/arch: amd64

tolerations: []

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        labelSelector:
          matchExpressions:
          - key: app.kubernetes.io/name
            operator: In
            values:
            - restapi-example
        topologyKey: kubernetes.io/hostname

# Deploy
helm install prod-api ./helm/restapi-example -f values-production.yaml -n production
```

### Vault Integration

```yaml
# values-vault.yaml
config:
  tls:
    enabled: true
    clientAuth: require
  auth:
    mode: mtls

vault:
  enabled: true
  address: "https://vault.example.com:8200"
  token: "hvs.CAESIJ..."
  pkiPath: "pki"
  pkiRole: "restapi-server"

# Deploy
helm install my-api ./helm/restapi-example -f values-vault.yaml
```

## Management Operations

### Upgrading

```bash
# Upgrade with new values
helm upgrade my-api ./helm/restapi-example -f my-values.yaml

# Upgrade to specific version
helm upgrade my-api ./helm/restapi-example --version 1.0.0

# Upgrade with rollback on failure
helm upgrade my-api ./helm/restapi-example --atomic --timeout 5m
```

### Rollback

```bash
# List releases
helm history my-api

# Rollback to previous version
helm rollback my-api

# Rollback to specific revision
helm rollback my-api 2
```

### Uninstalling

```bash
# Uninstall release
helm uninstall my-api

# Uninstall with cleanup
helm uninstall my-api --cascade=foreground
```

### Debugging

```bash
# Dry run to see generated manifests
helm install my-api ./helm/restapi-example --dry-run --debug

# Template rendering
helm template my-api ./helm/restapi-example -f my-values.yaml

# Get values
helm get values my-api

# Get manifest
helm get manifest my-api
```

## Monitoring and Observability

### Prometheus Metrics

When `serviceMonitor.enabled=true`, the chart creates a ServiceMonitor resource for Prometheus to scrape metrics:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  path: /metrics
  labels:
    release: prometheus  # Match your Prometheus operator labels
```

Available metrics:
- `http_requests_total` - Total HTTP requests
- `http_request_duration_seconds` - Request duration histogram
- `http_requests_in_flight` - Current requests being processed

### Health Checks

The deployment includes readiness and liveness probes:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 30
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: http
  initialDelaySeconds: 5
  periodSeconds: 5
```

## Security Considerations

### Pod Security

The chart implements security best practices:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 1000
  capabilities:
    drop:
    - ALL
```

### Network Security

For enhanced security, consider enabling NetworkPolicies:

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
      - namespaceSelector:
          matchLabels:
            name: ingress-nginx
      ports:
      - protocol: TCP
        port: 8080
```

### Secret Management

Sensitive configuration should use Kubernetes secrets:

```bash
# Create API key secret
kubectl create secret generic restapi-secrets \
  --from-literal=api-keys="key1:app1,key2:app2"

# Reference in values
config:
  apiKey:
    existingSecret: "restapi-secrets"
    secretKey: "api-keys"
```

## Troubleshooting

### Common Issues

1. **Pod not starting**: Check resource limits and node capacity
2. **Authentication failures**: Verify secret configuration and format
3. **TLS issues**: Ensure certificates are valid and properly mounted
4. **Ingress not working**: Check ingress controller and DNS configuration

### Debug Commands

```bash
# Check pod status
kubectl get pods -l app.kubernetes.io/name=restapi-example

# View pod logs
kubectl logs -l app.kubernetes.io/name=restapi-example

# Describe pod for events
kubectl describe pod <pod-name>

# Check service endpoints
kubectl get endpoints

# Test service connectivity
kubectl port-forward svc/my-api 8080:8080
curl http://localhost:8080/health
```

## Contributing

For chart improvements and bug reports, please refer to the main project repository.

## License

MIT License
