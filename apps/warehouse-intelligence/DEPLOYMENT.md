# Deployment Guide - Warehouse Intelligence System

## Table of Contents
1. [Local Development](#local-development)
2. [Docker Deployment](#docker-deployment)
3. [Kubernetes Deployment](#kubernetes-deployment)
4. [Database Setup](#database-setup)
5. [Monitoring & Alerts](#monitoring--alerts)
6. [Troubleshooting](#troubleshooting)

---

## Local Development

### Prerequisites
- Python 3.9+
- pip or poetry
- PostgreSQL 12+
- Redis (optional, for caching)

### Setup Steps

```bash
# 1. Clone repository
cd apps/warehouse-intelligence

# 2. Create virtual environment
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# 3. Install dependencies
pip install -r requirements.txt
pip install -e ".[dev,api]"

# 4. Create .env file
cat > .env << EOF
WAREHOUSE_ENV=development
DB_HOST=localhost
DB_PORT=5432
DB_NAME=warehouse_dev
DB_USER=postgres
DB_PASSWORD=postgres
PYTHONPATH=/path/to/warehouse-intelligence
EOF

# 5. Initialize database
python scripts/init_db.py

# 6. Run tests
pytest tests/ -v --cov=src

# 7. Start development server
python -m warehouse_intelligence.api
```

### Development Workflow

```bash
# Code formatting
black src/ tests/

# Linting
flake8 src/ tests/

# Type checking
mypy src/

# Full test suite with coverage
pytest tests/ --cov=src --cov-report=html
```

---

## Docker Deployment

### Build Image

```bash
# Build Docker image
docker build \
  -t warehouse-intelligence:1.0.0 \
  -f Dockerfile \
  .

# Build with BuildKit for better caching
DOCKER_BUILDKIT=1 docker build \
  -t warehouse-intelligence:1.0.0 \
  -f Dockerfile \
  .
```

### Dockerfile

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY src/ ./src/
COPY config.py .

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"

# Run application
CMD ["gunicorn", \
     "--workers", "4", \
     "--worker-class", "uvicorn.workers.UvicornWorker", \
     "--bind", "0.0.0.0:8000", \
     "--timeout", "120", \
     "--access-logfile", "-", \
     "--error-logfile", "-", \
     "warehouse_intelligence.api:app"]
```

### Docker Compose

```yaml
version: '3.8'

services:
  warehouse-intelligence:
    build:
      context: ./apps/warehouse-intelligence
      dockerfile: Dockerfile
    ports:
      - "8000:8000"
    environment:
      WAREHOUSE_ENV: development
      DB_HOST: postgres
      DB_PORT: 5432
      DB_NAME: warehouse_dev
      DB_USER: postgres
      DB_PASSWORD: postgres
      PYTHONUNBUFFERED: 1
    depends_on:
      postgres:
        condition: service_healthy
    networks:
      - warehouse
    volumes:
      - ./logs:/app/logs

  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: warehouse_dev
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./db/migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - warehouse

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    networks:
      - warehouse

volumes:
  postgres_data:

networks:
  warehouse:
    driver: bridge
```

### Run with Docker Compose

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f warehouse-intelligence

# Stop services
docker-compose down

# Rebuild images
docker-compose up -d --build
```

---

## Kubernetes Deployment

### Prerequisites
- Kubernetes cluster (1.24+)
- kubectl configured
- Helm 3.0+
- PostgreSQL operator or managed service

### Helm Installation

```bash
# Add Warehouse Intelligence Helm repo
helm repo add warehouse-intelligence https://charts.simaops.io
helm repo update

# Install release
helm install warehouse-intelligence warehouse-intelligence/warehouse-intelligence \
  --namespace warehouse-intelligence \
  --create-namespace \
  --values values.yaml
```

### Helm Values (values.yaml)

```yaml
replicaCount: 3

image:
  repository: warehouse-intelligence
  tag: "1.0.0"
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8000
  targetPort: 8000

ingress:
  enabled: true
  className: "nginx"
  annotations:
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
  hosts:
    - host: warehouse-intelligence.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: warehouse-intelligence-tls
      hosts:
        - warehouse-intelligence.example.com

resources:
  requests:
    memory: "512Mi"
    cpu: "250m"
  limits:
    memory: "2Gi"
    cpu: "1000m"

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

postgresql:
  auth:
    username: warehouse
    password: secure_password
    database: warehouse_prod
  persistence:
    size: 50Gi

environment:
  WAREHOUSE_ENV: "production"
  LOG_LEVEL: "INFO"
```

### Manual Kubernetes Deployment

```bash
# Create namespace
kubectl create namespace warehouse-intelligence

# Create ConfigMap
kubectl create configmap warehouse-config \
  --from-file=config.py \
  -n warehouse-intelligence

# Create Secrets
kubectl create secret generic warehouse-secrets \
  --from-literal=db-password=secure_password \
  --from-literal=api-key=your-api-key \
  -n warehouse-intelligence

# Deploy application
kubectl apply -f deployment.yaml -n warehouse-intelligence

# Check deployment
kubectl get deployments -n warehouse-intelligence
kubectl describe pod -n warehouse-intelligence
```

### Kubernetes Deployment Manifest

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: warehouse-intelligence
  namespace: warehouse-intelligence
  labels:
    app: warehouse-intelligence
spec:
  replicas: 3
  selector:
    matchLabels:
      app: warehouse-intelligence
  template:
    metadata:
      labels:
        app: warehouse-intelligence
    spec:
      containers:
      - name: warehouse-intelligence
        image: warehouse-intelligence:1.0.0
        ports:
        - containerPort: 8000
          name: http
        env:
        - name: WAREHOUSE_ENV
          value: "production"
        - name: DB_HOST
          value: "postgres.warehouse-intelligence.svc.cluster.local"
        - name: DB_PASSWORD
          valueFrom:
            secretKeyRef:
              name: warehouse-secrets
              key: db-password
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8000
          initialDelaySeconds: 10
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: warehouse-intelligence
  namespace: warehouse-intelligence
spec:
  selector:
    app: warehouse-intelligence
  type: ClusterIP
  ports:
  - protocol: TCP
    port: 8000
    targetPort: 8000
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: warehouse-intelligence-hpa
  namespace: warehouse-intelligence
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: warehouse-intelligence
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

---

## Database Setup

### PostgreSQL Schema

```bash
# Connect to PostgreSQL
psql -h localhost -U postgres -d warehouse_prod

# Run schema migrations
\i db/migrations/001_initial_schema.sql
\i db/migrations/002_warehouse_intelligence.sql
```

### Schema Migration Script

```sql
-- 002_warehouse_intelligence.sql

-- Slotting recommendations table
CREATE TABLE slotting_recommendations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id VARCHAR(255) NOT NULL,
    recommended_rack VARCHAR(255) NOT NULL,
    confidence_score FLOAT NOT NULL,
    associated_items TEXT[], -- JSON array of item IDs
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_item_id (item_id),
    INDEX idx_created_at (created_at)
);

-- Hazard validation audit trail
CREATE TABLE hazard_audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    audit_id VARCHAR(255) UNIQUE NOT NULL,
    item_id VARCHAR(255) NOT NULL,
    validation_status VARCHAR(50) NOT NULL,
    violations JSONB,
    rules_checked TEXT[],
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_item_id (item_id),
    INDEX idx_status (validation_status),
    INDEX idx_created_at (created_at)
);

-- Temperature sensor readings
CREATE TABLE sensor_readings (
    id BIGSERIAL PRIMARY KEY,
    sensor_id VARCHAR(100) NOT NULL,
    equipment_id VARCHAR(100) NOT NULL,
    temperature FLOAT NOT NULL,
    humidity FLOAT,
    compressor_status VARCHAR(50),
    timestamp TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_equipment_timestamp (equipment_id, timestamp),
    INDEX idx_sensor_timestamp (sensor_id, timestamp)
);

-- Cold-chain alerts
CREATE TABLE coldchain_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id VARCHAR(255) UNIQUE NOT NULL,
    equipment_id VARCHAR(100) NOT NULL,
    alert_type VARCHAR(100) NOT NULL,
    severity VARCHAR(50) NOT NULL,
    current_temperature FLOAT,
    message TEXT,
    remediation_actions JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_equipment_severity (equipment_id, severity),
    INDEX idx_created_at (created_at)
);

-- Create partitions for sensor_readings (time-series)
CREATE TABLE sensor_readings_2026_05 PARTITION OF sensor_readings
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');

CREATE TABLE sensor_readings_2026_06 PARTITION OF sensor_readings
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
```

---

## Monitoring & Alerts

### Prometheus Metrics

```bash
# Expose metrics on /metrics endpoint
# Example metrics:

warehouse_slotting_recommendations_total
warehouse_slotting_processing_duration_seconds
warehouse_hazard_validations_total
warehouse_hazard_validation_failures_total
warehouse_coldchain_alerts_total
warehouse_coldchain_sensor_readings_total
warehouse_coldchain_prediction_accuracy
warehouse_api_request_duration_seconds
warehouse_api_errors_total
```

### Grafana Dashboard

Create dashboard with panels:
- Slotting recommendations over time
- Hazard validation pass/fail rate
- Cold-chain alert trends
- Equipment health scores
- API latency p95/p99
- Error rates by module

### Alert Rules (Prometheus)

```yaml
groups:
- name: warehouse_intelligence
  rules:
  - alert: HighHazardValidationFailureRate
    expr: rate(warehouse_hazard_validation_failures_total[5m]) > 0.1
    for: 5m
    annotations:
      summary: "Hazard validation failure rate >10%"
      
  - alert: ColdChainCriticalAlerts
    expr: increase(warehouse_coldchain_alerts_total{severity="CRITICAL"}[5m]) > 2
    for: 1m
    annotations:
      summary: "Multiple critical cold-chain alerts"
      
  - alert: HighAPILatency
    expr: histogram_quantile(0.95, warehouse_api_request_duration_seconds) > 1
    for: 5m
    annotations:
      summary: "API p95 latency >1s"
```

---

## Troubleshooting

### Common Issues

#### 1. TensorFlow Installation Fails

```bash
# Solution: Install with specific version
pip install tensorflow==2.12.0 --no-build-isolation

# Or use CPU-only version (faster install)
pip install tensorflow-cpu==2.12.0
```

#### 2. Database Connection Timeout

```bash
# Check PostgreSQL is running
psql -h localhost -U postgres -c "SELECT 1"

# Verify connection string in config
echo $DB_HOST, $DB_PORT, $DB_NAME

# Increase connection pool
# In config.py: database.pool_size = 20
```

#### 3. High Memory Usage

```bash
# Monitor memory
docker stats warehouse-intelligence

# Solutions:
# 1. Reduce LSTM batch size (config.coldchain.lstm_batch_size = 16)
# 2. Implement time-series windowing
# 3. Use external time-series DB (InfluxDB)
```

#### 4. Slow LSTM Inference

```bash
# Check model size
ls -lh models/lstm_model.h5

# Solutions:
# 1. Use model quantization
# 2. Reduce LSTM units
# 3. Use TensorFlow Lite for edge deployment
```

### Debug Mode

```bash
# Enable debug logging
export WAREHOUSE_ENV=development
export LOG_LEVEL=DEBUG

# Run with verbose output
python -m warehouse_intelligence.api --debug

# Check logs
tail -f logs/warehouse_intelligence.log
```

---

## Performance Tuning

### Cold-Chain Monitoring

```python
# Optimize for latency
config.coldchain.lstm_batch_size = 1  # Real-time inference
config.coldchain.lstm_input_timesteps = 30  # Reduce history window

# Optimize for accuracy
config.coldchain.lstm_batch_size = 64  # Batch processing
config.coldchain.lstm_epochs = 200  # More training
```

### Database Optimization

```sql
-- Add indexes for common queries
CREATE INDEX idx_sensor_readings_equipment_time 
  ON sensor_readings (equipment_id, timestamp DESC);

CREATE INDEX idx_hazard_audit_item_time 
  ON hazard_audit_log (item_id, created_at DESC);

-- Analyze query performance
EXPLAIN ANALYZE
SELECT * FROM sensor_readings 
WHERE equipment_id = 'FRIDGE-A' 
  AND timestamp > NOW() - INTERVAL '24 hours';
```

---

## Production Checklist

- [ ] Environment variables configured
- [ ] Database migrations applied
- [ ] SSL/TLS certificates configured
- [ ] Monitoring and alerting enabled
- [ ] Backup strategy implemented
- [ ] Audit logging configured
- [ ] Load balancer configured
- [ ] CDN configured (if applicable)
- [ ] Rate limiting enabled
- [ ] CORS policy configured
- [ ] Security headers set
- [ ] Health checks implemented
- [ ] Graceful shutdown configured
- [ ] Log aggregation configured (ELK/Splunk)

---

## Maintenance

### Backup Strategy

```bash
# Daily PostgreSQL backup
0 2 * * * pg_dump warehouse_prod | gzip > /backups/warehouse_$(date +\%Y\%m\%d).sql.gz

# Retention: Keep 30 days of backups
find /backups -name "warehouse_*.sql.gz" -mtime +30 -delete
```

### Model Retraining

```bash
# Weekly LSTM model retraining
0 1 * * 1 python scripts/retrain_lstm_model.py

# Automatic model versioning
# New model: models/lstm_model_v2.h5
# Keep best 5 versions for rollback
```

---

**For additional support**: warehouse@simaops.io
