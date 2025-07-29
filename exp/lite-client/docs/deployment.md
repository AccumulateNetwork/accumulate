# Deployment Guide

[![Production Ready](https://img.shields.io/badge/production-ready-brightgreen.svg)](#production-deployment)
[![Docker Support](https://img.shields.io/badge/docker-supported-blue.svg)](#docker-deployment)
[![Cloud Native](https://img.shields.io/badge/cloud-native-purple.svg)](#cloud-deployment)

Complete deployment guide for the Accumulate Lite Client across different environments and platforms.

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [Environment Setup](#environment-setup)
- [Production Deployment](#production-deployment)
- [Docker Deployment](#docker-deployment)
- [Cloud Deployment](#cloud-deployment)
- [Configuration Management](#configuration-management)
- [Monitoring and Logging](#monitoring-and-logging)
- [Security Considerations](#security-considerations)
- [Troubleshooting](#troubleshooting)

## 🚀 Quick Start

### Local Development

```bash
# Clone the repository
git clone https://gitlab.com/accumulatenetwork/accumulate.git
cd accumulate/exp/lite-client

# Install dependencies
go mod download

# Run tests
go test ./...

# Build the application
go build -o lite-client ./cmd/lite-client

# Run with default configuration
./lite-client --network=testnet
```

### Using as a Library

```go
package main

import (
    "context"
    "log"
    
    liteclient "gitlab.com/accumulatenetwork/accumulate/exp/lite-client"
)

func main() {
    // Create client
    client, err := liteclient.NewClientWithNetwork("mainnet")
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    defer client.Close()
    
    // Use the client
    accounts, err := client.ProcessADIs(context.Background(), []string{
        "acc://my-adi.acme",
    })
    if err != nil {
        log.Fatal("Failed to process ADIs:", err)
    }
    
    log.Printf("Processed %d accounts", len(accounts))
}
```

## 🔧 Environment Setup

### System Requirements

| Component | Minimum | Recommended | Notes |
|-----------|---------|-------------|-------|
| **CPU** | 1 core | 2+ cores | Multi-core for parallel processing |
| **Memory** | 512MB | 2GB+ | Depends on cache size |
| **Storage** | 100MB | 1GB+ | For persistent cache |
| **Network** | 1Mbps | 10Mbps+ | For API communication |

### Go Environment

```bash
# Install Go 1.21 or later
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz

# Set up environment
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Verify installation
go version
```

### Dependencies

```bash
# Install required system packages
sudo apt-get update
sudo apt-get install -y \
    build-essential \
    git \
    curl \
    wget \
    ca-certificates

# Install optional monitoring tools
sudo apt-get install -y \
    htop \
    iotop \
    netstat-nat
```

## 🏭 Production Deployment

### Binary Deployment

#### 1. Build Production Binary

```bash
# Build optimized binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags)" \
    -o lite-client-linux-amd64 \
    ./cmd/lite-client

# Verify binary
./lite-client-linux-amd64 --version
```

#### 2. Create Deployment Structure

```bash
# Create application directory
sudo mkdir -p /opt/accumulate-lite-client/{bin,config,logs,cache}

# Copy binary
sudo cp lite-client-linux-amd64 /opt/accumulate-lite-client/bin/lite-client
sudo chmod +x /opt/accumulate-lite-client/bin/lite-client

# Create configuration
sudo tee /opt/accumulate-lite-client/config/production.json << EOF
{
  "network": {
    "server_url": "https://mainnet.accumulatenetwork.io:443",
    "backup_servers": [
      "https://backup1.accumulatenetwork.io:443",
      "https://backup2.accumulatenetwork.io:443"
    ],
    "timeout": "30s",
    "retry_attempts": 3,
    "retry_delay": "1s"
  },
  "cache": {
    "ttl": "15m",
    "max_entries": 10000,
    "persistent_cache": true,
    "cache_dir": "/opt/accumulate-lite-client/cache"
  },
  "api": {
    "max_concurrency": 20,
    "rate_limit": 100,
    "request_timeout": "30s"
  },
  "debug": {
    "enable_logging": true,
    "log_level": "info",
    "verbose_errors": false
  }
}
EOF
```

#### 3. Create Systemd Service

```bash
# Create service file
sudo tee /etc/systemd/system/accumulate-lite-client.service << EOF
[Unit]
Description=Accumulate Lite Client
Documentation=https://docs.accumulatenetwork.io/
After=network.target
Wants=network.target

[Service]
Type=simple
User=accumulate
Group=accumulate
ExecStart=/opt/accumulate-lite-client/bin/lite-client \\
    --config=/opt/accumulate-lite-client/config/production.json \\
    --log-file=/opt/accumulate-lite-client/logs/lite-client.log
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=accumulate-lite-client

# Security settings
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/accumulate-lite-client/cache /opt/accumulate-lite-client/logs

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

# Create user
sudo useradd --system --home /opt/accumulate-lite-client --shell /bin/false accumulate
sudo chown -R accumulate:accumulate /opt/accumulate-lite-client

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable accumulate-lite-client
sudo systemctl start accumulate-lite-client
sudo systemctl status accumulate-lite-client
```

### Load Balancer Setup

#### Nginx Configuration

```nginx
upstream accumulate_lite_client {
    least_conn;
    server 127.0.0.1:8080 max_fails=3 fail_timeout=30s;
    server 127.0.0.1:8081 max_fails=3 fail_timeout=30s;
    server 127.0.0.1:8082 max_fails=3 fail_timeout=30s;
}

server {
    listen 80;
    listen 443 ssl http2;
    server_name lite-client.example.com;

    # SSL configuration
    ssl_certificate /etc/ssl/certs/lite-client.crt;
    ssl_certificate_key /etc/ssl/private/lite-client.key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512;

    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload";

    # Rate limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req zone=api burst=20 nodelay;

    location / {
        proxy_pass http://accumulate_lite_client;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 30s;
        proxy_send_timeout 30s;
        proxy_read_timeout 30s;
        
        # Health check
        proxy_next_upstream error timeout invalid_header http_500 http_502 http_503 http_504;
    }

    # Health check endpoint
    location /health {
        proxy_pass http://accumulate_lite_client/health;
        access_log off;
    }

    # Metrics endpoint (restricted)
    location /metrics {
        allow 10.0.0.0/8;
        allow 172.16.0.0/12;
        allow 192.168.0.0/16;
        deny all;
        proxy_pass http://accumulate_lite_client/metrics;
    }
}
```

## 🐳 Docker Deployment

### Dockerfile

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always)" \
    -o lite-client \
    ./cmd/lite-client

# Final stage
FROM scratch

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary
COPY --from=builder /app/lite-client /lite-client

# Create non-root user
USER 65534:65534

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/lite-client", "health-check"]

# Run application
ENTRYPOINT ["/lite-client"]
```

### Docker Compose

```yaml
version: '3.8'

services:
  lite-client:
    build: .
    image: accumulate/lite-client:latest
    container_name: accumulate-lite-client
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - ACCUMULATE_NETWORK=mainnet
      - ACCUMULATE_SERVER_URL=https://mainnet.accumulatenetwork.io:443
      - ACCUMULATE_CACHE_TTL=15m
      - ACCUMULATE_CACHE_DIR=/data/cache
      - ACCUMULATE_LOG_LEVEL=info
    volumes:
      - ./data/cache:/data/cache
      - ./data/logs:/data/logs
      - ./config:/config:ro
    networks:
      - accumulate-network
    healthcheck:
      test: ["CMD", "/lite-client", "health-check"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

  # Optional: Redis for distributed caching
  redis:
    image: redis:7-alpine
    container_name: accumulate-redis
    restart: unless-stopped
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    networks:
      - accumulate-network
    command: redis-server --appendonly yes

  # Optional: Prometheus for monitoring
  prometheus:
    image: prom/prometheus:latest
    container_name: accumulate-prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    networks:
      - accumulate-network
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
      - '--web.console.libraries=/etc/prometheus/console_libraries'
      - '--web.console.templates=/etc/prometheus/consoles'

networks:
  accumulate-network:
    driver: bridge

volumes:
  redis-data:
  prometheus-data:
```

### Build and Deploy

```bash
# Build image
docker build -t accumulate/lite-client:latest .

# Run with docker-compose
docker-compose up -d

# Check status
docker-compose ps
docker-compose logs lite-client

# Scale instances
docker-compose up -d --scale lite-client=3
```

## ☁️ Cloud Deployment

### AWS ECS Deployment

#### Task Definition

```json
{
  "family": "accumulate-lite-client",
  "networkMode": "awsvpc",
  "requiresCompatibilities": ["FARGATE"],
  "cpu": "512",
  "memory": "1024",
  "executionRoleArn": "arn:aws:iam::ACCOUNT:role/ecsTaskExecutionRole",
  "taskRoleArn": "arn:aws:iam::ACCOUNT:role/ecsTaskRole",
  "containerDefinitions": [
    {
      "name": "lite-client",
      "image": "accumulate/lite-client:latest",
      "essential": true,
      "portMappings": [
        {
          "containerPort": 8080,
          "protocol": "tcp"
        }
      ],
      "environment": [
        {
          "name": "ACCUMULATE_NETWORK",
          "value": "mainnet"
        },
        {
          "name": "ACCUMULATE_SERVER_URL",
          "value": "https://mainnet.accumulatenetwork.io:443"
        }
      ],
      "secrets": [
        {
          "name": "API_KEY",
          "valueFrom": "arn:aws:secretsmanager:region:account:secret:accumulate/api-key"
        }
      ],
      "logConfiguration": {
        "logDriver": "awslogs",
        "options": {
          "awslogs-group": "/ecs/accumulate-lite-client",
          "awslogs-region": "us-west-2",
          "awslogs-stream-prefix": "ecs"
        }
      },
      "healthCheck": {
        "command": [
          "CMD-SHELL",
          "/lite-client health-check"
        ],
        "interval": 30,
        "timeout": 5,
        "retries": 3,
        "startPeriod": 60
      }
    }
  ]
}
```

#### Service Definition

```json
{
  "serviceName": "accumulate-lite-client",
  "cluster": "accumulate-cluster",
  "taskDefinition": "accumulate-lite-client",
  "desiredCount": 3,
  "launchType": "FARGATE",
  "networkConfiguration": {
    "awsvpcConfiguration": {
      "subnets": [
        "subnet-12345678",
        "subnet-87654321"
      ],
      "securityGroups": [
        "sg-12345678"
      ],
      "assignPublicIp": "ENABLED"
    }
  },
  "loadBalancers": [
    {
      "targetGroupArn": "arn:aws:elasticloadbalancing:region:account:targetgroup/accumulate-tg/1234567890123456",
      "containerName": "lite-client",
      "containerPort": 8080
    }
  ],
  "serviceRegistries": [
    {
      "registryArn": "arn:aws:servicediscovery:region:account:service/srv-12345678"
    }
  ]
}
```

### Kubernetes Deployment

#### Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: accumulate-lite-client
  namespace: accumulate
  labels:
    app: lite-client
    version: v1.0.0
spec:
  replicas: 3
  selector:
    matchLabels:
      app: lite-client
  template:
    metadata:
      labels:
        app: lite-client
        version: v1.0.0
    spec:
      containers:
      - name: lite-client
        image: accumulate/lite-client:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: ACCUMULATE_NETWORK
          value: "mainnet"
        - name: ACCUMULATE_SERVER_URL
          value: "https://mainnet.accumulatenetwork.io:443"
        - name: ACCUMULATE_CACHE_DIR
          value: "/data/cache"
        envFrom:
        - configMapRef:
            name: lite-client-config
        - secretRef:
            name: lite-client-secrets
        volumeMounts:
        - name: cache-volume
          mountPath: /data/cache
        - name: config-volume
          mountPath: /config
          readOnly: true
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        securityContext:
          runAsNonRoot: true
          runAsUser: 65534
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
      volumes:
      - name: cache-volume
        persistentVolumeClaim:
          claimName: lite-client-cache
      - name: config-volume
        configMap:
          name: lite-client-config
      securityContext:
        fsGroup: 65534
---
apiVersion: v1
kind: Service
metadata:
  name: accumulate-lite-client
  namespace: accumulate
  labels:
    app: lite-client
spec:
  selector:
    app: lite-client
  ports:
  - port: 80
    targetPort: 8080
    name: http
  type: ClusterIP
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: accumulate-lite-client
  namespace: accumulate
  annotations:
    kubernetes.io/ingress.class: nginx
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/rate-limit: "100"
spec:
  tls:
  - hosts:
    - lite-client.example.com
    secretName: lite-client-tls
  rules:
  - host: lite-client.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: accumulate-lite-client
            port:
              number: 80
```

## ⚙️ Configuration Management

### Environment Variables

```bash
# Network configuration
export ACCUMULATE_NETWORK=mainnet
export ACCUMULATE_SERVER_URL=https://mainnet.accumulatenetwork.io:443
export ACCUMULATE_BACKUP_SERVERS=https://backup1.example.com,https://backup2.example.com

# Cache configuration
export ACCUMULATE_CACHE_TTL=15m
export ACCUMULATE_CACHE_MAX_ENTRIES=10000
export ACCUMULATE_CACHE_DIR=/data/cache
export ACCUMULATE_PERSISTENT_CACHE=true

# Performance configuration
export ACCUMULATE_MAX_CONCURRENT=20
export ACCUMULATE_RATE_LIMIT=100
export ACCUMULATE_REQUEST_TIMEOUT=30s

# Debug configuration
export ACCUMULATE_DEBUG=false
export ACCUMULATE_LOG_LEVEL=info
export ACCUMULATE_VERBOSE_ERRORS=false

# Security configuration
export ACCUMULATE_API_KEY=your-api-key-here
export ACCUMULATE_TLS_CERT_FILE=/etc/ssl/certs/lite-client.crt
export ACCUMULATE_TLS_KEY_FILE=/etc/ssl/private/lite-client.key
```

### Configuration Files

#### Production Configuration (`production.json`)

```json
{
  "network": {
    "server_url": "https://mainnet.accumulatenetwork.io:443",
    "backup_servers": [
      "https://backup1.accumulatenetwork.io:443",
      "https://backup2.accumulatenetwork.io:443"
    ],
    "timeout": "30s",
    "retry_attempts": 3,
    "retry_delay": "1s"
  },
  "cache": {
    "ttl": "15m",
    "max_entries": 10000,
    "persistent_cache": true,
    "cache_dir": "/data/cache"
  },
  "api": {
    "max_concurrency": 20,
    "rate_limit": 100,
    "request_timeout": "30s"
  },
  "debug": {
    "enable_logging": true,
    "log_level": "info",
    "verbose_errors": false
  }
}
```

#### Development Configuration (`development.json`)

```json
{
  "network": {
    "server_url": "https://testnet.accumulatenetwork.io:443",
    "timeout": "10s",
    "retry_attempts": 1,
    "retry_delay": "500ms"
  },
  "cache": {
    "ttl": "5m",
    "max_entries": 1000,
    "persistent_cache": false
  },
  "api": {
    "max_concurrency": 5,
    "rate_limit": 50,
    "request_timeout": "10s"
  },
  "debug": {
    "enable_logging": true,
    "log_level": "debug",
    "verbose_errors": true
  }
}
```

## 📊 Monitoring and Logging

### Prometheus Metrics

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'accumulate-lite-client'
    static_configs:
      - targets: ['lite-client:8080']
    metrics_path: /metrics
    scrape_interval: 30s
```

### Grafana Dashboard

```json
{
  "dashboard": {
    "title": "Accumulate Lite Client",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(accumulate_requests_total[5m])",
            "legendFormat": "Requests/sec"
          }
        ]
      },
      {
        "title": "Cache Hit Rate",
        "type": "stat",
        "targets": [
          {
            "expr": "accumulate_cache_hit_rate",
            "legendFormat": "Hit Rate"
          }
        ]
      },
      {
        "title": "Response Time",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, accumulate_request_duration_seconds_bucket)",
            "legendFormat": "95th percentile"
          }
        ]
      }
    ]
  }
}
```

### Log Configuration

```yaml
# logrotate configuration
/opt/accumulate-lite-client/logs/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    create 0644 accumulate accumulate
    postrotate
        systemctl reload accumulate-lite-client
    endscript
}
```

## 🔒 Security Considerations

### Network Security

```bash
# Firewall rules (iptables)
iptables -A INPUT -p tcp --dport 8080 -s 10.0.0.0/8 -j ACCEPT
iptables -A INPUT -p tcp --dport 8080 -j DROP

# Or using ufw
ufw allow from 10.0.0.0/8 to any port 8080
ufw deny 8080
```

### TLS Configuration

```bash
# Generate self-signed certificate for testing
openssl req -x509 -newkey rsa:4096 -keyout lite-client.key -out lite-client.crt -days 365 -nodes

# Or use Let's Encrypt
certbot certonly --standalone -d lite-client.example.com
```

### Secret Management

```bash
# Using Kubernetes secrets
kubectl create secret generic lite-client-secrets \
    --from-literal=api-key=your-api-key \
    --from-literal=db-password=your-db-password

# Using AWS Secrets Manager
aws secretsmanager create-secret \
    --name accumulate/lite-client/api-key \
    --secret-string your-api-key
```

## 🔧 Troubleshooting

### Common Issues

#### 1. Connection Timeouts

```bash
# Check network connectivity
curl -v https://mainnet.accumulatenetwork.io:443/health

# Check DNS resolution
nslookup mainnet.accumulatenetwork.io

# Check firewall rules
iptables -L -n
```

#### 2. Memory Issues

```bash
# Check memory usage
free -h
ps aux | grep lite-client

# Check cache size
du -sh /data/cache

# Monitor memory in real-time
top -p $(pgrep lite-client)
```

#### 3. Performance Issues

```bash
# Check CPU usage
htop

# Check I/O usage
iotop

# Check network usage
netstat -i
ss -tuln
```

### Debug Commands

```bash
# Enable debug logging
export ACCUMULATE_LOG_LEVEL=debug

# Check configuration
./lite-client --config-check

# Health check
./lite-client health-check

# Version information
./lite-client --version

# Cache statistics
curl http://localhost:8080/cache/stats
```

### Log Analysis

```bash
# View recent logs
journalctl -u accumulate-lite-client -f

# Search for errors
grep -i error /opt/accumulate-lite-client/logs/lite-client.log

# Analyze performance
grep "request_duration" /opt/accumulate-lite-client/logs/lite-client.log | \
    awk '{print $NF}' | sort -n | tail -10
```

## 📞 Support

For deployment support and troubleshooting:

- **Documentation**: [docs/](../docs/)
- **Examples**: [examples/](../examples/)
- **Issues**: [GitHub Issues](https://github.com/AccumulateNetwork/accumulate/issues)
- **Community**: [Discord](https://discord.gg/accumulate)
- **Professional Support**: [Contact Accumulate](https://accumulatenetwork.io/contact)

---

This deployment guide provides comprehensive instructions for deploying the Accumulate Lite Client in various environments. Choose the deployment method that best fits your infrastructure and requirements.
