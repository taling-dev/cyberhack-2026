# Warehouse Intelligence System - Production Dockerfile

FROM python:3.11-slim as builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    libpq-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and pre-install dependencies
COPY requirements.txt .
RUN pip install --user --no-cache-dir -r requirements.txt

# ============================================================================
# Production Stage
# ============================================================================

FROM python:3.11-slim

WORKDIR /app

# Install runtime dependencies only
RUN apt-get update && apt-get install -y \
    libpq5 \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -m -u 1000 warehouse && \
    mkdir -p /app/logs && \
    chown -R warehouse:warehouse /app

# Copy Python packages from builder
COPY --from=builder /root/.local /home/warehouse/.local

# Set environment variables
ENV PATH=/home/warehouse/.local/bin:$PATH \
    PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    WAREHOUSE_ENV=production

# Copy application code
COPY --chown=warehouse:warehouse src/ ./src/
COPY --chown=warehouse:warehouse config.py .
COPY --chown=warehouse:warehouse ARCHITECTURE.md .

# Switch to non-root user
USER warehouse

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Run application with gunicorn + uvicorn workers
CMD ["gunicorn", \
     "--workers", "4", \
     "--worker-class", "uvicorn.workers.UvicornWorker", \
     "--bind", "0.0.0.0:8000", \
     "--timeout", "120", \
     "--graceful-timeout", "30", \
     "--keep-alive", "5", \
     "--access-logfile", "-", \
     "--error-logfile", "-", \
     "--log-level", "info", \
     "warehouse_intelligence.api:app"]

# Labels for container metadata
LABEL maintainer="warehouse@simaops.io" \
      version="1.0.0" \
      description="Warehouse Intelligence System - Smart warehouse management with ML, rule-based, and LSTM modules"
