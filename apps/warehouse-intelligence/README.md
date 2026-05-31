# Warehouse Intelligence System

A comprehensive AI-powered warehouse management platform for fresh produce supply chains, featuring machine learning, rule-based compliance, and deep learning predictive monitoring.

## Overview

Warehouse Intelligence consists of three core modules that work together to optimize warehouse operations:

### 1. **Smart Slotting & Market Basket Analysis** 🤖
Uses **FP-Growth/Apriori** algorithms to analyze historical order patterns and recommend optimal item placement based on co-picking frequencies.

- **Objective**: Reduce picking time and warehouse congestion
- **Algorithm**: FP-Growth (Frequent Pattern Mining)
- **Latency**: 5-10 minutes (batch processing)
- **Output**: Warehouse slotting recommendations with confidence scores

### 2. **Hazard Segregation** 🚨
Deterministic rule-based system ensuring compliance with INI/IBC regulations for hazardous material segregation.

- **Objective**: Ensure safe storage and regulatory compliance
- **Approach**: Decision matrices (NO machine learning)
- **Latency**: < 100ms (real-time validation)
- **Compliance**: INI, IBC, FDA, OSHA standards
- **Features**: Audit trails, remediation suggestions

### 3. **Cold-Chain Monitoring** ❄️
LSTM-based predictive system for temperature anomaly detection and early warning alerts.

- **Objective**: Prevent produce degradation through predictive maintenance
- **Algorithm**: LSTM (Long Short-Term Memory) neural network
- **Latency**: < 500ms (real-time inference)
- **Input**: IoT sensor data (temperature, humidity, compressor status)
- **Output**: Current + predicted alerts with remediation actions

---

## Architecture

```
┌─────────────────────────────────────────────────┐
│   Warehouse Management API (Go Backend)         │
├─────────────────────────────────────────────────┤
│  Endpoints:                                     │
│  - POST /api/warehouse/slotting/optimize       │
│  - POST /api/warehouse/hazard/validate         │
│  - POST /api/warehouse/coldchain/process       │
│  - GET /api/warehouse/system/status            │
└────────────────────┬────────────────────────────┘
                     │
        ┌────────────┼────────────┐
        │            │            │
   ┌────▼─────┐ ┌───▼────┐  ┌───▼──────────┐
   │ Slotting │ │ Hazard │  │ Cold-Chain   │
   │ Engine   │ │ Engine │  │ Monitoring   │
   │ (Python) │ │(Python)│  │ (Python/LSTM)│
   └────┬─────┘ └───┬────┘  └───┬──────────┘
        │           │            │
        └───────────┼────────────┘
                    │
         ┌──────────▼──────────┐
         │  Event Outbox       │
         │  PostgreSQL + NATS  │
         └─────────────────────┘
```

---

## Installation

### Prerequisites
- Python 3.9+
- pip or poetry
- PostgreSQL (for audit trails)
- Optional: InfluxDB/TimescaleDB (for time-series data)

### Setup

```bash
# Clone repository
cd apps/warehouse-intelligence

# Install dependencies
pip install -r requirements.txt

# For development with tests
pip install -e ".[dev]"

# For API endpoints
pip install -e ".[api]"
```

---

## Quick Start

### 1. Smart Slotting Example

```python
from warehouse_intelligence.src.smart_slotting import (
    SmartSlottingEngine, Transaction
)
from datetime import datetime

# Initialize engine
engine = SmartSlottingEngine(
    min_support=0.02,      # 2% of transactions
    min_confidence=0.3,    # 30% confidence threshold
    min_lift=1.5          # 1.5x lift threshold
)

# Load historical order data
transactions = [
    Transaction(
        order_id="ORD-001",
        items=["APPLE", "BANANA", "ORANGE"],
        timestamp=datetime.now(),
        quantity_map={"APPLE": 10, "BANANA": 15, "ORANGE": 8}
    ),
    # ... more transactions
]

# Run full pipeline
recommendations = engine.run_full_pipeline(transactions)

# Results
for rec in recommendations:
    print(f"{rec.item_id} → Rack: {rec.recommended_rack}")
    print(f"  Associated items: {rec.associated_items}")
    print(f"  Confidence: {rec.confidence_score:.2%}\n")
```

### 2. Hazard Segregation Example

```python
from warehouse_intelligence.src.hazard_segregation import (
    HazardSegregationEngine, Item, StorageLocation, 
    AdjacencyInfo, HazardLevel, StorageCategory
)

# Initialize engine
engine = HazardSegregationEngine()

# Define items and locations
pesticide = Item(
    item_id="PST-001",
    name="Glyphosate Herbicide",
    hazard_level=HazardLevel.PESTICIDE,
    compliance_standard="INI"
)

storage_location = StorageLocation(
    rack_id="HAZMAT-05",
    category=StorageCategory.PESTICIDE_STORAGE,
    zone="H",
    temperature=20.0
)

adjacent_fresh = AdjacencyInfo(
    rack_id="PRODUCE-01",
    distance_meters=3,
    category=StorageCategory.FRESH_PRODUCE,
    items=[Item("APPLE-001", "Apples", HazardLevel.SAFE, "FDA")]
)

# Validate placement
result = engine.validate_item_placement(
    pesticide, storage_location, [adjacent_fresh]
)

print(f"Status: {result.status.value}")
print(f"Violations: {result.violations}")
print(f"Remediation: {result.remediation_actions}")

# Export audit report
report = engine.export_compliance_report(result)
```

### 3. Cold-Chain Monitoring Example

```python
from warehouse_intelligence.src.cold_chain_monitoring import (
    ColdChainMonitoringEngine, SensorReading
)

# Initialize engine (LSTM model auto-initialized)
engine = ColdChainMonitoringEngine()

# Process sensor reading
reading = SensorReading(
    sensor_id="SENSOR-01",
    equipment_id="FRIDGE-A",
    temperature=-3.5,
    humidity=75.0,
    compressor_status="ON",
    ambient_temperature=25.0,
    timestamp=datetime.now()
)

# Process and check for alerts
alert = engine.process_sensor_reading(reading)

if alert:
    print(f"[{alert.severity.value}] {alert.alert_type.value}")
    print(f"Temperature: {alert.current_temperature}°C")
    print(f"Message: {alert.message}")
    print(f"Recommended Actions:")
    for action in alert.remediation_actions:
        print(f"  - {action}")

# Check equipment health
health = engine.get_equipment_health_score("FRIDGE-A")
print(f"\nEquipment Health: {health['health_score']}/100")
```

---

## Module Details

### Smart Slotting Module

**Key Metrics:**
- Support: % of transactions containing itemset
- Confidence: P(Item B | Item A picked)
- Lift: Association strength (>1.0 = positive correlation)

**Output:**
- Rack assignment recommendations
- Associated item groupings
- Placement distance guidelines

**Use Cases:**
- Optimize warehouse layout for faster picking
- Reduce travel distance between frequently co-picked items
- Seasonal product placement adjustments

### Hazard Segregation Module

**Decision Matrix Rules:**
| Hazard Level | Required Distance | Prohibited Adjacent | Ventilation |
|---|---|---|---|
| PESTICIDE | 5m | FOOD, BEVERAGE | HIGH |
| TOXIC | 8m | FOOD, BEVERAGE, REFRIGERATED | VERY_HIGH |
| FLAMMABLE | 10m | HAZMAT, PESTICIDE | HIGH |

**Audit Trail:**
- Rule ID that triggered decision
- Timestamp and audit UUID
- Violations and recommendations
- Export-ready compliance reports

### Cold-Chain Monitoring Module

**LSTM Architecture:**
```
Input (60 timesteps × 4 features)
  ↓
[LSTM-128] → Dropout(0.2)
  ↓
[LSTM-64] → Dropout(0.2)
  ↓
[Dense-32, ReLU] → [Dense-16, ReLU]
  ↓
Output: Predicted temps (next 5 timesteps)
```

**Alert Types:**
- `CURRENT_THRESHOLD`: Immediate violation
- `PREDICTED_ANOMALY`: LSTM forecast violation (2+ hours ahead)
- `TREND_DEGRADATION`: Temperature trending wrong direction
- `EQUIPMENT_FAILURE`: Sensor malfunction detected

---

## API Integration

### Warehouse API Endpoints

#### 1. Optimize Slotting
```bash
POST /api/warehouse/slotting/optimize
Content-Type: application/json

{
  "transactions": [
    {
      "order_id": "ORD-001",
      "items": ["APPLE", "BANANA"],
      "timestamp": "2026-05-31T10:00:00Z",
      "quantity_map": {"APPLE": 10, "BANANA": 15}
    }
  ]
}

Response:
{
  "success": true,
  "data": {
    "recommendations": [...],
    "association_rules": [...],
    "metrics": {
      "transactions_analyzed": 1000,
      "itemsets_discovered": 45,
      "rules_generated": 12
    }
  }
}
```

#### 2. Validate Hazard Placement
```bash
POST /api/warehouse/hazard/validate
Content-Type: application/json

{
  "item": {
    "item_id": "PST-001",
    "name": "Glyphosate",
    "hazard_level": "PESTICIDE",
    "compliance_standard": "INI"
  },
  "location": {
    "rack_id": "HAZMAT-05",
    "category": "PESTICIDE_STORAGE",
    "temperature": 20.0
  },
  "adjacent_locations": [...]
}

Response:
{
  "success": true,
  "data": {
    "status": "REJECTED",
    "is_approved": false,
    "audit_report": {...},
    "remediation": [...]
  }
}
```

#### 3. Process Cold-Chain Sensor
```bash
POST /api/warehouse/coldchain/process
Content-Type: application/json

{
  "sensor_id": "SENSOR-01",
  "equipment_id": "FRIDGE-A",
  "temperature": -3.5,
  "timestamp": "2026-05-31T10:00:00Z"
}

Response:
{
  "success": true,
  "data": {
    "equipment_id": "FRIDGE-A",
    "current_temperature": -3.5,
    "alert": null,
    "equipment_health": {
      "equipment_id": "FRIDGE-A",
      "health_score": 95,
      "status": "HEALTHY"
    }
  }
}
```

---

## Testing

```bash
# Run all tests
pytest tests/ -v

# Run with coverage
pytest tests/ --cov=src

# Run specific module tests
pytest tests/test_smart_slotting.py -v
pytest tests/test_hazard_segregation.py -v
pytest tests/test_cold_chain_monitoring.py -v
```

---

## Performance & Scalability

| Component | Throughput | Latency | Storage |
|---|---|---|---|
| Smart Slotting | 10K+ transactions/run | 5-10 min | Minimal (in-memory) |
| Hazard Segregation | 10K+ validations/hour | < 100ms | Audit logs (~1KB/validation) |
| Cold-Chain Monitor | 1000+ sensors | < 500ms inference | Time-series DB (~50KB/day/sensor) |

---

## Deployment

### Docker

```dockerfile
FROM python:3.11-slim

WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt

COPY src/ ./src/
COPY config/ ./config/

CMD ["python", "-m", "warehouse_intelligence.api"]
```

### Kubernetes

See [deploy/k8s/warehouse-intelligence](../../deploy/k8s/) for Helm charts.

---

## Compliance & Standards

- ✅ INI (Ikatan Nasional Indonesia) - Hazmat regulations
- ✅ IBC (Intermediate Bulk Container) - Container standards
- ✅ FDA - Food storage requirements
- ✅ OSHA - Occupational safety
- ✅ ISO 22000 - Food safety management
- ✅ IFS - International featured standards

---

## Development

### Code Quality

```bash
# Format code
black src/

# Lint
flake8 src/

# Type checking
mypy src/
```

### Contributing

1. Create feature branch: `git checkout -b feature/new-module`
2. Write tests for new functionality
3. Ensure all tests pass: `pytest`
4. Submit pull request

---

## Troubleshooting

### TensorFlow Installation Issues
```bash
# If TensorFlow fails to install:
pip install tensorflow==2.12.0 --no-build-isolation
```

### LSTM Model Not Loading
```python
# Falls back to mock model if TensorFlow unavailable
# Check logs for warnings
import logging
logging.basicConfig(level=logging.DEBUG)
```

### High Memory Usage
- Reduce LSTM batch size in config
- Implement time-series data windowing
- Use InfluxDB instead of in-memory storage

---

## License

MIT License - See LICENSE file

---

## Support & Contact

- **Issues**: [GitHub Issues](https://github.com/simaops/warehouse-intelligence/issues)
- **Documentation**: [Full Docs](https://warehouse-intelligence.simaops.io)
- **Email**: warehouse@simaops.io

---

**Version**: 1.0.0  
**Last Updated**: May 31, 2026
