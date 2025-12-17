# MessageDB Deployment Guide

This guide covers deploying MessageDB in production environments.

## Table of Contents

- [Docker Deployment](#docker-deployment)
- [Docker Compose](#docker-compose)
- [Kubernetes](#kubernetes)
- [Configuration](#configuration)
- [Security](#security)
- [Monitoring](#monitoring)
- [Backup & Recovery](#backup--recovery)
- [High Availability](#high-availability)

---

## Docker Deployment

### Building the Docker Image

```bash
# From the project root
docker build -t messagedb:latest .
```

### Running with Docker

```bash
# Basic run (in-memory SQLite)
docker run -d \
  --name messagedb \
  -p 8080:8080 \
  messagedb:latest

# With custom token
docker run -d \
  --name messagedb \
  -p 8080:8080 \
  -e MESSAGEDB_TOKEN=your-secure-token \
  messagedb:latest
```

### Docker Image Details

The image is a multi-stage build:
- **Builder stage**: Go 1.21 Alpine, compiles the binary
- **Runtime stage**: Alpine Linux, minimal footprint (~15MB)

---

## Docker Compose

### Basic Setup

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  messagedb:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MESSAGEDB_PORT=8080
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
    restart: unless-stopped
```

### With PostgreSQL

For production deployments with PostgreSQL:

```yaml
version: '3.8'

services:
  messagedb:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MESSAGEDB_DB_HOST=postgres
      - MESSAGEDB_DB_PORT=5432
      - MESSAGEDB_DB_NAME=message_store
      - MESSAGEDB_DB_USER=message_store
      - MESSAGEDB_DB_PASSWORD=${DB_PASSWORD}
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
    restart: unless-stopped

  postgres:
    image: postgres:14-alpine
    environment:
      - POSTGRES_DB=message_store
      - POSTGRES_USER=message_store
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U message_store -d message_store"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

volumes:
  pgdata:
```

### Starting the Stack

```bash
# Create .env file
echo "DB_PASSWORD=your-secure-password" > .env

# Start services
docker compose up -d

# View logs
docker compose logs -f messagedb

# Stop services
docker compose down
```

---

## Kubernetes

### Deployment Manifest

Create `k8s/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: messagedb
  labels:
    app: messagedb
spec:
  replicas: 1
  selector:
    matchLabels:
      app: messagedb
  template:
    metadata:
      labels:
        app: messagedb
    spec:
      containers:
      - name: messagedb
        image: messagedb:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: MESSAGEDB_PORT
          value: "8080"
        - name: MESSAGEDB_DB_HOST
          valueFrom:
            secretKeyRef:
              name: messagedb-secrets
              key: db-host
        - name: MESSAGEDB_DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: messagedb-secrets
              key: db-password
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 1000m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
```

### Service Manifest

Create `k8s/service.yaml`:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: messagedb
  labels:
    app: messagedb
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: messagedb
```

### Ingress (Optional)

Create `k8s/ingress.yaml`:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: messagedb
  annotations:
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
spec:
  rules:
  - host: messagedb.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: messagedb
            port:
              number: 8080
```

### Deploy to Kubernetes

```bash
# Create namespace
kubectl create namespace messagedb

# Create secrets
kubectl create secret generic messagedb-secrets \
  --namespace=messagedb \
  --from-literal=db-host=postgres.database.svc.cluster.local \
  --from-literal=db-password=your-password

# Apply manifests
kubectl apply -f k8s/ --namespace=messagedb

# Check status
kubectl get pods -n messagedb
kubectl logs -f deployment/messagedb -n messagedb
```

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MESSAGEDB_PORT` | 8080 | HTTP server port |
| `MESSAGEDB_TOKEN` | (generated) | Default namespace token |
| `MESSAGEDB_TEST_MODE` | false | Enable test mode |
| `MESSAGEDB_DB_HOST` | - | PostgreSQL host |
| `MESSAGEDB_DB_PORT` | 5432 | PostgreSQL port |
| `MESSAGEDB_DB_NAME` | message_store | PostgreSQL database |
| `MESSAGEDB_DB_USER` | message_store | PostgreSQL user |
| `MESSAGEDB_DB_PASSWORD` | - | PostgreSQL password |

### Command Line Flags

```bash
./messagedb serve --help

Flags:
  --port int        HTTP server port (default 8080)
  --test-mode       Run in test mode (in-memory SQLite)
  --token string    Token for default namespace
```

### Recommended Production Settings

```yaml
# docker-compose.yml
environment:
  # Server configuration
  - MESSAGEDB_PORT=8080
  
  # PostgreSQL connection
  - MESSAGEDB_DB_HOST=postgres
  - MESSAGEDB_DB_PORT=5432
  - MESSAGEDB_DB_NAME=message_store
  - MESSAGEDB_DB_USER=message_store
  - MESSAGEDB_DB_PASSWORD=${DB_PASSWORD}
  
  # Connection pool settings
  - MESSAGEDB_DB_MAX_CONNECTIONS=50
  - MESSAGEDB_DB_IDLE_CONNECTIONS=10
```

---

## Security

### Token Management

1. **Generate secure tokens** for each namespace:
   ```bash
   # Token format: ns_<base64url-namespace>_<random>
   # Use cryptographically secure random suffix
   ```

2. **Rotate tokens** periodically:
   - Create new token with `ns.create` (custom token option)
   - Update clients to use new token
   - Delete old namespace if needed

3. **Store tokens securely**:
   - Use secrets management (Vault, AWS Secrets Manager, K8s Secrets)
   - Never commit tokens to version control
   - Use environment variables in production

### Network Security

1. **TLS Termination**: Use a reverse proxy (nginx, Traefik) for HTTPS
   ```nginx
   server {
       listen 443 ssl;
       server_name messagedb.example.com;
       
       ssl_certificate /etc/ssl/certs/messagedb.crt;
       ssl_certificate_key /etc/ssl/private/messagedb.key;
       
       location / {
           proxy_pass http://messagedb:8080;
           proxy_http_version 1.1;
           proxy_set_header Upgrade $http_upgrade;
           proxy_set_header Connection "upgrade";
           proxy_read_timeout 3600s;
       }
   }
   ```

2. **Network Policies** (Kubernetes):
   ```yaml
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: messagedb-policy
   spec:
     podSelector:
       matchLabels:
         app: messagedb
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: my-app
       ports:
       - port: 8080
   ```

### Audit Logging

Enable request logging for security auditing:

```bash
# Logs include timestamp, method, path, status, duration
2024-01-15T10:30:00Z INFO  POST /rpc 200 15ms
```

---

## Monitoring

### Health Checks

```bash
# HTTP health endpoint
curl http://localhost:8080/health
# {"status":"ok"}

# Version endpoint
curl http://localhost:8080/version
# {"version":"1.3.0"}
```

### Prometheus Metrics

(Coming in future version)

Planned metrics:
- `messagedb_requests_total` - Total RPC requests
- `messagedb_request_duration_seconds` - Request latency histogram
- `messagedb_messages_written_total` - Messages written
- `messagedb_sse_connections` - Active SSE connections

### Logging

MessageDB logs to stdout in JSON format:

```json
{"level":"info","ts":"2024-01-15T10:30:00Z","msg":"Server starting","port":8080,"version":"1.3.0"}
{"level":"info","ts":"2024-01-15T10:30:01Z","msg":"Request","method":"stream.write","stream":"account-123","duration":"5ms"}
```

Configure log aggregation (ELK, Loki, CloudWatch) for production.

---

## Backup & Recovery

### SQLite (Test Mode)

In test mode, data is stored in memory and lost on restart. Not suitable for production.

### PostgreSQL

Use standard PostgreSQL backup strategies:

```bash
# Logical backup
pg_dump -h localhost -U message_store message_store > backup.sql

# Point-in-time recovery with WAL archiving
# Configure in postgresql.conf:
archive_mode = on
archive_command = 'cp %p /backup/wal/%f'

# Restore
psql -h localhost -U message_store message_store < backup.sql
```

### Automated Backups (Kubernetes)

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: messagedb-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:14-alpine
            command:
            - /bin/sh
            - -c
            - |
              pg_dump -h $DB_HOST -U $DB_USER $DB_NAME | \
              gzip > /backup/messagedb-$(date +%Y%m%d).sql.gz
            env:
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: messagedb-secrets
                  key: db-password
          restartPolicy: OnFailure
```

---

## High Availability

### Single Instance (Simple)

For many use cases, a single MessageDB instance with PostgreSQL is sufficient:
- Use managed PostgreSQL (RDS, Cloud SQL) for database HA
- Rely on container orchestration for server restarts

### Multiple Instances (Scaling)

MessageDB instances are stateless and can be horizontally scaled:

```yaml
# Kubernetes deployment with multiple replicas
spec:
  replicas: 3
```

**Note**: SSE subscriptions are per-instance. Use a message broker for cross-instance notifications in high-scale scenarios.

### Load Balancing

Use a load balancer with session affinity for SSE connections:

```yaml
# Kubernetes service with session affinity
apiVersion: v1
kind: Service
metadata:
  name: messagedb
spec:
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600
```

---

## Troubleshooting

### Common Issues

1. **Connection refused**
   - Check if server is running: `curl http://localhost:8080/health`
   - Verify port mapping in Docker/K8s
   - Check firewall rules

2. **Authentication errors**
   - Verify token format: `ns_<base64>_<random>`
   - Check token matches namespace
   - Ensure `Authorization: Bearer <token>` header

3. **Database connection errors**
   - Verify PostgreSQL is running
   - Check connection string parameters
   - Review PostgreSQL logs

4. **SSE connections dropping**
   - Increase proxy timeouts
   - Check network stability
   - Monitor server resources

### Debug Mode

Enable verbose logging:

```bash
# Set log level
MESSAGEDB_LOG_LEVEL=debug ./messagedb serve
```

### Getting Help

- Check server logs: `docker logs messagedb`
- Verify configuration: `docker inspect messagedb`
- Test connectivity: `curl -v http://localhost:8080/health`
