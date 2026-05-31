# Smart Warehouse Intelligence System - Logical Architecture

## Overview
This system provides three core modules for fresh produce warehouse optimization:
1. **Smart Slotting & Market Basket Analysis** - ML-driven location optimization
2. **Hazard Segregation** - Compliance-based rule enforcement
3. **Cold-Chain Monitoring** - LSTM-based temperature prediction

---

## Module 1: Smart Slotting & Market Basket Analysis

### Logical Architecture
```
Order History Data
    ↓
Data Normalization & Cleaning
    ↓
Transaction Format Transformation
    ↓
FP-Growth/Apriori Algorithm
    ↓
Association Rules Generation
    (Item A → Item B: confidence=0.85, support=0.12)
    ↓
Slot Optimization Engine
    ↓
Warehouse Floor Plan Recommendations
```

### Key Concepts
- **Transaction**: A single order containing multiple items
- **Item Set**: Combination of items frequently purchased together
- **Support**: Percentage of transactions containing the item set
- **Confidence**: Likelihood of Item B given Item A was picked
- **Lift**: Strength of the association rule (Lift > 1 = strong correlation)

### Data Flow
```
Input: Orders table (order_id, item_id, timestamp, quantity)
    ↓
Aggregation: {Transaction: [item1, item2, item3], ...}
    ↓
FP-Growth Mining: {Rules: [(itemA, itemB, confidence, support, lift)]}
    ↓
Output: Slotting recommendations with rack assignments
```

### Integration Points
- **Input**: PostgreSQL orders table via `apps/api/internal/db`
- **Processing**: Batch job (daily/weekly) via `apps/ai-worker`
- **Output**: Store recommendations in `recommendations` table, publish via outbox-publisher
- **Triggers**: Slotting recommendations via events system

---

## Module 2: Hazard Segregation

### Logical Architecture
```
Item Details (name, category, hazard_level)
    ↓
Current Rack Configuration
    ↓
Decision Matrix Evaluation
    ↓
Rule-Based Validation Engine
    ↓
Compliance Status (APPROVED / REJECTED / NEEDS_REVIEW)
    ↓
Audit Log & Error Tracing
```

### Decision Matrix Structure
```
IF item_hazard_level IN ['HAZMAT', 'PESTICIDE', 'TOXIC'] 
  AND adjacent_rack_category IN ['FOOD', 'BEVERAGE', 'FRESH_PRODUCE']
  THEN status = REJECTED, reason = "Hazmat segregation violation"

IF item_hazard_level = 'HAZMAT'
  AND storage_distance < MIN_SEGREGATION_DISTANCE (meters)
  THEN status = REJECTED, reason = "Insufficient segregation distance"

IF item_hazard_level = 'REFRIGERATED'
  AND ambient_temp > MAX_TEMP_THRESHOLD
  THEN status = REJECTED, reason = "Temperature control failure"
```

### Compliance Rules (INI/IBC Regulation)
- Hazardous materials must be ≥ 5 meters from food items
- Separate ventilation required for hazmat zones
- Emergency access routes must be maintained
- Segregated storage areas with secondary containment

### Data Flow
```
Input: (item, proposed_rack, adjacent_racks, storage_conditions)
    ↓
Hazard Classification Lookup
    ↓
Adjacent Item Category Check
    ↓
Distance & Segregation Validation
    ↓
Output: {status: APPROVED/REJECTED, reason: string, audit_id: UUID}
```

### Integration Points
- **Input**: Item master data + rack assignments from API
- **Processing**: Real-time validation (< 100ms response time)
- **Output**: Audit trail for compliance reporting
- **Triggers**: Pre-storage validation, placement verification

---

## Module 3: Cold-Chain Monitoring

### Logical Architecture
```
IoT Temperature Sensors (15-minute intervals)
    ↓
Raw Data Ingestion & Validation
    ↓
Time-Series Feature Engineering
    ↓
LSTM Model Inference
    ↓
Anomaly Detection & Prediction
    ↓
Alert Generation System
    ↓
Remediation Workflow
```

### LSTM Architecture
```
Input Layer: (batch_size, time_steps=60, features=4)
    - Current temperature
    - Temperature change rate (derivative)
    - Ambient external temperature
    - Compressor status (on/off)

Hidden Layers:
    - LSTM Cell 1: 128 units, return_sequences=True
    - Dropout: 0.2 (prevent overfitting)
    - LSTM Cell 2: 64 units, return_sequences=False
    - Dropout: 0.2

Dense Layers:
    - Dense: 32 units, ReLU activation
    - Dense: 1 unit, Linear activation (temperature prediction)

Output: Predicted temperature for next timestep
```

### Alert Logic
```
Current Alert (Immediate):
  IF current_temp < -22°C OR current_temp > -2°C
    THEN CRITICAL_ALERT
  
Predictive Alert (Early Warning):
  LSTM_prediction shows temp will exceed thresholds within 2 hours
    THEN WARNING_ALERT + Recommended actions (defrost cycle, compressor check)

Trend Alert (Degradation):
  IF rate_of_change > 1°C/hour (unexpected warming)
    THEN ANOMALY_ALERT + Investigate cause
```

### Data Flow
```
Input: Time-series sensor data (sensor_id, temp, timestamp, equipment_id)
    ↓
Data Validation (remove outliers, interpolate gaps)
    ↓
Sliding Window Creation (60 timesteps)
    ↓
Feature Engineering (rate of change, external temp)
    ↓
LSTM Inference (predict next 5 timesteps)
    ↓
Alert Threshold Evaluation
    ↓
Output: {alert_type, severity, predicted_temps, recommendations}
```

### Integration Points
- **Input**: MQTT/Kafka streams from IoT sensors
- **Processing**: Real-time inference (< 500ms latency)
- **Storage**: Time-series DB (InfluxDB/TimescaleDB)
- **Output**: Alert events via outbox-publisher
- **Triggers**: SMS/Email/Dashboard notifications

---

## System Integration Overview

```
┌─────────────────────────────────────────────────────────┐
│         Warehouse Management API (Go)                   │
├─────────────────────────────────────────────────────────┤
│  ├─ /api/slotting/recommendations                       │
│  ├─ /api/hazard/validate-placement                      │
│  └─ /api/coldchain/sensor-status                        │
└─────────────────────────────────────────────────────────┘
                          ↑
        ┌─────────────────┼─────────────────┐
        │                 │                 │
┌───────▼────────┐  ┌────▼──────────┐  ┌──▼─────────────┐
│ Smart Slotting │  │ Hazard        │  │ Cold-Chain     │
│ (Python Worker)│  │ Segregation   │  │ Monitoring     │
│ - FP-Growth    │  │ (Rules Engine)│  │ (LSTM Model)   │
│ - Batch Job    │  │ - Real-time   │  │ - Real-time    │
└────────┬───────┘  └────┬──────────┘  └──┬─────────────┘
         │                │                 │
         └────────────────┼─────────────────┘
                          ↓
                  ┌──────────────────┐
                  │  Outbox Publisher│
                  │  Event System    │
                  └──────────────────┘
                          ↓
                  PostgreSQL + Events
```

---

## Data Models

### Transactions (Smart Slotting)
```python
@dataclass
class Transaction:
    order_id: str
    items: List[str]
    timestamp: datetime
    quantity_map: Dict[str, int]
```

### Hazard Rules
```python
@dataclass
class HazardRule:
    hazard_level: str  # HAZMAT, PESTICIDE, TOXIC, REFRIGERATED
    segregation_distance: int  # meters
    adjacent_prohibited_categories: List[str]
    compliance_standard: str  # INI, IBC, FDA
```

### Sensor Reading
```python
@dataclass
class SensorReading:
    sensor_id: str
    equipment_id: str
    temperature: float
    humidity: float
    timestamp: datetime
```

---

## Performance Targets

| Module | Operation | Latency | Accuracy |
|--------|-----------|---------|----------|
| Smart Slotting | Batch recommendation generation | 5-10 min | 80%+ itemset coverage |
| Hazard Segregation | Real-time validation | < 100ms | 99%+ compliance rate |
| Cold-Chain Monitoring | LSTM inference | < 500ms | 85%+ prediction accuracy |

---

## Compliance & Auditing

- All rule rejections logged with audit trail (Rule ID, Timestamp, Reason)
- LSTM predictions stored for model explainability
- Association rules tracked for slotting change history
- Audit logs exportable for compliance reporting (ISO 22000, IFS)

