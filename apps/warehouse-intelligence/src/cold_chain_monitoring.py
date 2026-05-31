"""
Cold-Chain Monitoring Module - LSTM-Based Predictive Alerts

Implements Deep Learning (LSTM) for time-series temperature prediction
in cold-chain warehouses for fresh produce. Provides proactive alerts
based on temperature trend forecasting.

Features:
- Anomaly detection (current temperature thresholds)
- Predictive alerts (future temperature deviations)
- Root cause analysis (trend patterns)
- Maintenance recommendations

Author: Warehouse Intelligence System
Version: 1.0
"""

from dataclasses import dataclass, field
from typing import List, Dict, Tuple, Optional
from enum import Enum
from datetime import datetime, timedelta
import numpy as np
import logging
from abc import ABC, abstractmethod

# For production: use TensorFlow/Keras
# pip install tensorflow numpy scikit-learn
try:
    import tensorflow as tf
    from tensorflow import keras
    from tensorflow.keras.layers import LSTM, Dense, Dropout
    from tensorflow.keras.optimizers import Adam
    HAS_TENSORFLOW = True
except ImportError:
    HAS_TENSORFLOW = False
    logging.warning("TensorFlow not installed. Using mock LSTM implementation.")


# ============================================================================
# Enums & Constants
# ============================================================================

class AlertSeverity(Enum):
    """Alert severity levels."""
    INFO = "INFO"
    WARNING = "WARNING"
    CRITICAL = "CRITICAL"
    MAINTENANCE = "MAINTENANCE"


class AlertType(Enum):
    """Types of cold-chain alerts."""
    CURRENT_THRESHOLD = "CURRENT_THRESHOLD"  # Current temp out of range
    PREDICTED_ANOMALY = "PREDICTED_ANOMALY"  # LSTM predicts future violation
    TREND_DEGRADATION = "TREND_DEGRADATION"  # Temperature trending wrong direction
    EQUIPMENT_FAILURE = "EQUIPMENT_FAILURE"  # Likely compressor/sensor issue
    DOOR_OPEN = "DOOR_OPEN"  # Extended door opening detected
    HIGH_AMBIENT = "HIGH_AMBIENT"  # Ambient temperature too high


# Temperature thresholds (°C) for fresh produce
TEMPERATURE_THRESHOLDS = {
    "OPTIMAL_MIN": -4.0,
    "OPTIMAL_MAX": -2.0,
    "ACCEPTABLE_MIN": -22.0,
    "ACCEPTABLE_MAX": 0.0,
    "CRITICAL_MIN": -25.0,
    "CRITICAL_MAX": 2.0,
}

# Sensor sampling interval
SENSOR_SAMPLING_INTERVAL_MINUTES = 15


# ============================================================================
# Data Models
# ============================================================================

@dataclass
class SensorReading:
    """Raw temperature sensor measurement."""
    sensor_id: str
    equipment_id: str  # Cold room/freezer ID
    temperature: float
    humidity: Optional[float] = None
    compressor_status: Optional[str] = None  # "ON", "OFF", "ERROR"
    door_status: Optional[str] = None  # "OPEN", "CLOSED"
    ambient_temperature: Optional[float] = None
    timestamp: datetime = field(default_factory=datetime.now)
    is_anomaly: bool = False  # Sensor error flag


@dataclass
class TimeSeriesFeatures:
    """Engineered time-series features for LSTM model."""
    temperature_sequence: np.ndarray  # Shape: (timesteps,)
    rate_of_change: np.ndarray  # Temperature derivative
    ambient_temp_sequence: np.ndarray
    compressor_status_sequence: np.ndarray  # Binary: 1=ON, 0=OFF
    timestamp: datetime


@dataclass
class LSTMPrediction:
    """LSTM model prediction output."""
    predicted_temperatures: List[float]  # Next 5 timesteps
    prediction_timestamps: List[datetime]
    confidence_score: float  # 0-1, based on input variance
    will_violate_threshold: bool
    violation_timeframe: Optional[timedelta] = None
    recommended_actions: List[str] = field(default_factory=list)


@dataclass
class ColdChainAlert:
    """Alert triggered by cold-chain monitoring system."""
    alert_id: str
    equipment_id: str
    alert_type: AlertType
    severity: AlertSeverity
    current_temperature: float
    message: str
    timestamp: datetime
    triggered_by_rule: str  # Rule ID that triggered this alert
    predicted_data: Optional[LSTMPrediction] = None
    remediation_actions: List[str] = field(default_factory=list)
    requires_immediate_action: bool = False


# ============================================================================
# LSTM Model Implementation
# ============================================================================

class ColdChainLSTMModel:
    """
    LSTM-based time-series forecasting for cold-chain temperature prediction.
    
    Architecture:
    - Input: 60 timesteps (15 hours of data at 15-min intervals)
    - Features per timestep: [temp, rate_of_change, ambient_temp, compressor_status]
    - Two LSTM layers with dropout for regularization
    - Output: Predicted temperature for next 5 timesteps
    
    Hyperparameters (tuned for fresh produce):
    - LSTM Units: [128, 64] (captures complex temporal patterns)
    - Dropout: 0.2 (prevents overfitting on small datasets)
    - Batch Size: 32 (stable gradients)
    - Epochs: 100 (convergence with early stopping)
    """
    
    def __init__(self, input_shape: Tuple[int, int] = (60, 4)):
        """
        Initialize LSTM model.
        
        Args:
            input_shape: (timesteps, features)
                - timesteps: 60 (15-hour history)
                - features: 4 (temp, rate_of_change, ambient_temp, compressor_status)
        """
        self.input_shape = input_shape
        self.model = None
        self.scaler_mean = 0.0
        self.scaler_std = 1.0
        self.logger = logging.getLogger(__name__)
        
        if HAS_TENSORFLOW:
            self._build_model()
        else:
            self.logger.warning("Using mock LSTM model (TensorFlow not available)")
    
    def _build_model(self):
        """Build LSTM neural network architecture."""
        if not HAS_TENSORFLOW:
            return
        
        model = keras.Sequential([
            # First LSTM layer: 128 units, returns sequences for stacking
            LSTM(128, activation='relu', return_sequences=True, 
                 input_shape=self.input_shape, name='lstm_1'),
            Dropout(0.2, name='dropout_1'),
            
            # Second LSTM layer: 64 units
            LSTM(64, activation='relu', return_sequences=False, name='lstm_2'),
            Dropout(0.2, name='dropout_2'),
            
            # Dense layers for prediction
            Dense(32, activation='relu', name='dense_1'),
            Dense(16, activation='relu', name='dense_2'),
            
            # Output layer: Predict next 5 timesteps
            Dense(5, activation='linear', name='output')
        ])
        
        model.compile(
            optimizer=Adam(learning_rate=0.001),
            loss='mse',
            metrics=['mae']
        )
        
        self.model = model
        self.logger.info("LSTM model built successfully")
    
    def train(self, X_train: np.ndarray, y_train: np.ndarray, 
              epochs: int = 100, batch_size: int = 32, validation_split: float = 0.2):
        """
        Train LSTM model on historical temperature data.
        
        Args:
            X_train: Training sequences, shape (samples, timesteps, features)
            y_train: Target values (next 5 temperatures), shape (samples, 5)
            epochs: Number of training epochs
            batch_size: Batch size for gradient updates
            validation_split: Fraction of data for validation
        """
        if not HAS_TENSORFLOW:
            self.logger.warning("Mock LSTM: skipping training")
            return
        
        self.logger.info(f"Training LSTM model for {epochs} epochs...")
        
        # Normalize data
        self.scaler_mean = np.mean(X_train)
        self.scaler_std = np.std(X_train)
        X_train_normalized = (X_train - self.scaler_mean) / (self.scaler_std + 1e-8)
        
        early_stopping = keras.callbacks.EarlyStopping(
            monitor='val_loss',
            patience=10,
            restore_best_weights=True
        )
        
        history = self.model.fit(
            X_train_normalized, y_train,
            epochs=epochs,
            batch_size=batch_size,
            validation_split=validation_split,
            callbacks=[early_stopping],
            verbose=1
        )
        
        self.logger.info(f"Training completed. Final loss: {history.history['loss'][-1]:.6f}")
        return history
    
    def predict(self, X: np.ndarray) -> np.ndarray:
        """
        Predict future temperatures.
        
        Args:
            X: Input sequences, shape (samples, timesteps, features)
        
        Returns:
            Predicted temperatures for next 5 timesteps, shape (samples, 5)
        """
        if not HAS_TENSORFLOW:
            # Mock prediction: return slightly warmed temperatures
            return np.random.normal(-5, 1, (X.shape[0], 5))
        
        X_normalized = (X - self.scaler_mean) / (self.scaler_std + 1e-8)
        return self.model.predict(X_normalized, verbose=0)
    
    def get_model_summary(self) -> str:
        """Get model architecture summary."""
        if not self.model:
            return "Mock LSTM model (TensorFlow not available)"
        
        import io
        import sys
        
        string_buffer = io.StringIO()
        self.model.summary(print_fn=lambda x: string_buffer.write(x + '\n'))
        return string_buffer.getvalue()


# ============================================================================
# Cold-Chain Monitoring Engine
# ============================================================================

class ColdChainMonitoringEngine:
    """
    Real-time cold-chain monitoring with LSTM-based predictive alerts.
    
    Features:
    1. Current threshold monitoring (immediate alerts)
    2. LSTM prediction (early warnings)
    3. Trend analysis (degradation detection)
    4. Equipment health assessment (compressor/sensor diagnostics)
    
    Alert Generation:
    - Current violation: temp outside safe range → CRITICAL_ALERT
    - Predicted violation: LSTM forecast shows breach in 2 hours → WARNING_ALERT
    - Trend alert: temp warming at >1°C/hour → ANOMALY_ALERT
    """
    
    def __init__(self, lstm_model: Optional[ColdChainLSTMModel] = None):
        """
        Initialize monitoring engine.
        
        Args:
            lstm_model: Trained LSTM model (if None, creates untrained instance)
        """
        self.lstm_model = lstm_model or ColdChainLSTMModel()
        self.sensor_history: Dict[str, List[SensorReading]] = {}
        self.alerts: List[ColdChainAlert] = []
        self.logger = logging.getLogger(__name__)
        # Per-equipment threshold overrides (by equipment_id). Falls back to the
        # global fresh-produce TEMPERATURE_THRESHOLDS when an id is absent.
        self.equipment_thresholds: Dict[str, dict] = {}

    def _thresholds(self, equipment_id: str) -> dict:
        return self.equipment_thresholds.get(equipment_id, TEMPERATURE_THRESHOLDS)
    
    def process_sensor_reading(self, reading: SensorReading) -> Optional[ColdChainAlert]:
        """
        Process single sensor reading and check for alerts.
        
        Steps:
        1. Validate sensor reading
        2. Check current temperature thresholds
        3. Compile time-series features
        4. Run LSTM prediction
        5. Analyze trends
        6. Generate alerts if needed
        
        Args:
            reading: Raw sensor measurement
        
        Returns:
            Alert if triggered, None otherwise
        """
        # Initialize sensor history if needed
        if reading.sensor_id not in self.sensor_history:
            self.sensor_history[reading.sensor_id] = []
        
        # Validate sensor reading
        if not self._validate_sensor_reading(reading):
            return self._create_alert(
                reading.equipment_id,
                AlertType.EQUIPMENT_FAILURE,
                AlertSeverity.CRITICAL,
                reading.temperature,
                "Sensor reading validation failed (possible sensor error)"
            )
        
        # Store in history
        self.sensor_history[reading.sensor_id].append(reading)
        
        # Rule 1: Check current temperature thresholds
        alert = self._check_current_thresholds(reading)
        if alert:
            return alert
        
        # Need at least 60 readings for LSTM (15 hours)
        if len(self.sensor_history[reading.sensor_id]) < 60:
            return None
        
        # Rule 2: LSTM prediction for early warning
        alert = self._predict_temperature_violation(reading)
        if alert:
            return alert
        
        # Rule 3: Trend analysis
        alert = self._analyze_temperature_trend(reading)
        if alert:
            return alert
        
        return None  # No alert
    
    def _validate_sensor_reading(self, reading: SensorReading) -> bool:
        """Validate sensor reading for data quality."""
        # Check temperature range (realistic for cold storage)
        if not (-30 < reading.temperature < 50):
            self.logger.warning(f"Temperature out of realistic range: {reading.temperature}°C")
            return False
        
        # Check for stuck readings (same value for extended period)
        if len(self.sensor_history.get(reading.sensor_id, [])) > 10:
            recent = self.sensor_history[reading.sensor_id][-10:]
            if all(abs(r.temperature - reading.temperature) < 0.1 for r in recent):
                self.logger.warning(f"Sensor {reading.sensor_id} may be stuck")
                return False
        
        return True
    
    def _check_current_thresholds(self, reading: SensorReading) -> Optional[ColdChainAlert]:
        """Rule 1: Check if current temperature violates thresholds."""
        
        th = self._thresholds(reading.equipment_id)
        # Critical violation
        if reading.temperature < th["CRITICAL_MIN"] or \
           reading.temperature > th["CRITICAL_MAX"]:
            return self._create_alert(
                reading.equipment_id,
                AlertType.CURRENT_THRESHOLD,
                AlertSeverity.CRITICAL,
                reading.temperature,
                f"Temperature {reading.temperature}°C exceeds critical limits",
                requires_immediate_action=True,
                actions=[
                    "Check compressor status immediately",
                    "Verify emergency backup systems are functioning",
                    "Consider emergency product relocation"
                ]
            )
        
        # Warning - outside acceptable range
        if reading.temperature < th["ACCEPTABLE_MIN"] or \
           reading.temperature > th["ACCEPTABLE_MAX"]:
            return self._create_alert(
                reading.equipment_id,
                AlertType.CURRENT_THRESHOLD,
                AlertSeverity.WARNING,
                reading.temperature,
                f"Temperature {reading.temperature}°C outside acceptable range",
                actions=[
                    "Schedule equipment inspection",
                    "Monitor temperature closely for changes"
                ]
            )
        
        return None
    
    def _predict_temperature_violation(self, reading: SensorReading) -> Optional[ColdChainAlert]:
        """Rule 2: Use LSTM to predict future temperature violations."""
        
        # Prepare input for LSTM (last 60 readings)
        history = self.sensor_history[reading.sensor_id][-60:]
        
        if len(history) < 60:
            return None
        
        # Build feature matrix
        features = self._build_lstm_features(history)
        
        # Run prediction
        prediction = self._run_lstm_prediction(features)
        
        if prediction.will_violate_threshold:
            return self._create_alert(
                reading.equipment_id,
                AlertType.PREDICTED_ANOMALY,
                AlertSeverity.WARNING,
                reading.temperature,
                f"LSTM prediction: Temperature will violate thresholds in "
                f"{prediction.violation_timeframe.total_seconds() / 3600:.1f} hours",
                predicted_data=prediction,
                actions=prediction.recommended_actions,
                requires_immediate_action=False
            )
        
        return None
    
    def _analyze_temperature_trend(self, reading: SensorReading) -> Optional[ColdChainAlert]:
        """Rule 3: Detect temperature trending in wrong direction."""
        
        history = self.sensor_history[reading.sensor_id][-10:]  # Last 2.5 hours
        
        if len(history) < 5:
            return None
        
        # Calculate rate of change (°C per hour)
        temps = [r.temperature for r in history]
        time_hours = (history[-1].timestamp - history[0].timestamp).total_seconds() / 3600
        
        if time_hours < 0.1:  # Avoid division by very small number
            return None
        
        rate_of_change = (temps[-1] - temps[0]) / time_hours
        
        # Alert if warming too rapidly (away from target)
        if rate_of_change > 1.0:  # More than 1°C/hour warming
            return self._create_alert(
                reading.equipment_id,
                AlertType.TREND_DEGRADATION,
                AlertSeverity.WARNING,
                reading.temperature,
                f"Temperature trending upward at {rate_of_change:.2f}°C/hour",
                actions=[
                    "Verify compressor is running (check compressor_status)",
                    "Inspect for door leaks or prolonged door openings",
                    "Check ambient temperature around equipment"
                ]
            )
        
        return None
    
    def _build_lstm_features(self, history: List[SensorReading]) -> TimeSeriesFeatures:
        """Build feature matrix from sensor history for LSTM."""
        
        temps = np.array([r.temperature for r in history])
        
        # Calculate rate of change (derivative)
        rate_of_change = np.diff(temps, prepend=temps[0])
        
        # Ambient temperature (interpolate if missing)
        ambient = np.array([r.ambient_temperature or np.mean(temps) for r in history])
        
        # Compressor status (1 = ON, 0 = OFF)
        compressor = np.array([1.0 if (r.compressor_status == "ON") else 0.0 for r in history])
        
        # Stack features: (timesteps, features)
        feature_matrix = np.column_stack([temps, rate_of_change, ambient, compressor])
        
        return TimeSeriesFeatures(
            temperature_sequence=temps,
            rate_of_change=rate_of_change,
            ambient_temp_sequence=ambient,
            compressor_status_sequence=compressor,
            timestamp=history[-1].timestamp
        )
    
    def _run_lstm_prediction(self, features: TimeSeriesFeatures) -> LSTMPrediction:
        """Run LSTM model inference on features."""
        
        # Prepare input: add batch dimension and reshape
        X = features.temperature_sequence.reshape(1, 60, 4)
        
        # Get model predictions
        predicted_temps = self.lstm_model.predict(X)[0]
        
        # Generate prediction timestamps (15-min intervals)
        prediction_times = []
        for i in range(1, 6):
            pred_time = features.timestamp + timedelta(minutes=i * 15)
            prediction_times.append(pred_time)
        
        # Check if any prediction violates thresholds
        will_violate = any(
            temp < TEMPERATURE_THRESHOLDS["ACCEPTABLE_MIN"] or 
            temp > TEMPERATURE_THRESHOLDS["ACCEPTABLE_MAX"]
            for temp in predicted_temps
        )
        
        # Calculate violation timeframe
        violation_time = None
        if will_violate:
            for i, temp in enumerate(predicted_temps):
                if temp < TEMPERATURE_THRESHOLDS["ACCEPTABLE_MIN"] or \
                   temp > TEMPERATURE_THRESHOLDS["ACCEPTABLE_MAX"]:
                    violation_time = timedelta(minutes=(i + 1) * 15)
                    break
        
        # Calculate confidence (inverse of input variance)
        input_variance = np.var(features.temperature_sequence)
        confidence = 1.0 / (1.0 + input_variance)
        
        # Generate recommendations
        recommendations = [
            "Review compressor maintenance schedule",
            "Check door seals for leaks",
            f"Predicted next reading: {predicted_temps[0]:.1f}°C"
        ]
        
        return LSTMPrediction(
            predicted_temperatures=list(predicted_temps),
            prediction_timestamps=prediction_times,
            confidence_score=confidence,
            will_violate_threshold=will_violate,
            violation_timeframe=violation_time,
            recommended_actions=recommendations
        )
    
    def _create_alert(
        self,
        equipment_id: str,
        alert_type: AlertType,
        severity: AlertSeverity,
        current_temp: float,
        message: str,
        requires_immediate_action: bool = False,
        actions: Optional[List[str]] = None,
        predicted_data: Optional[LSTMPrediction] = None
    ) -> ColdChainAlert:
        """Create and store alert."""
        import uuid
        
        alert = ColdChainAlert(
            alert_id=str(uuid.uuid4()),
            equipment_id=equipment_id,
            alert_type=alert_type,
            severity=severity,
            current_temperature=current_temp,
            message=message,
            timestamp=datetime.now(),
            triggered_by_rule=f"{alert_type.value}",
            predicted_data=predicted_data,
            remediation_actions=actions or [],
            requires_immediate_action=requires_immediate_action
        )
        
        self.alerts.append(alert)
        self.logger.warning(f"ALERT TRIGGERED: {alert.alert_type.value} ({alert.severity.value})")
        
        return alert
    
    def get_equipment_health_score(self, equipment_id: str) -> Dict:
        """
        Assess equipment health based on sensor history.
        
        Returns score 0-100 and recommendations.
        """
        score = 100
        issues = []
        
        # Find all readings for this equipment
        equipment_readings = []
        for sensor_history in self.sensor_history.values():
            equipment_readings.extend([r for r in sensor_history if r.equipment_id == equipment_id])
        
        if not equipment_readings:
            return {"score": 0, "status": "NO_DATA", "issues": ["No readings available"]}
        
        # Check stability (low variance = good)
        recent = equipment_readings[-60:]
        temps = [r.temperature for r in recent]
        variance = np.var(temps)
        
        if variance > 2.0:
            score -= 20
            issues.append("High temperature variance (possible thermostat issue)")
        elif variance > 1.0:
            score -= 10
            issues.append("Moderate temperature variance")
        
        # Check recent violations
        th = self._thresholds(equipment_id)
        violations = sum(1 for r in recent if not (
            th["ACCEPTABLE_MIN"] <= r.temperature <= th["ACCEPTABLE_MAX"]
        ))
        
        if violations > 5:
            score -= 30
            issues.append("Multiple recent temperature violations")
        elif violations > 0:
            score -= 15
            issues.append(f"{violations} recent temperature excursions")
        
        return {
            "equipment_id": equipment_id,
            "health_score": max(0, score),
            "status": "CRITICAL" if score < 40 else "WARNING" if score < 70 else "HEALTHY",
            "issues": issues
        }


# ============================================================================
# Example Usage
# ============================================================================

def example_cold_chain_monitoring():
    """Demonstration of Cold-Chain Monitoring System."""
    
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - [%(name)s] - %(levelname)s - %(message)s'
    )
    
    print("\n" + "="*80)
    print("COLD-CHAIN MONITORING SYSTEM - LSTM PREDICTIVE ALERTS")
    print("="*80 + "\n")
    
    # Initialize monitoring engine
    lstm_model = ColdChainLSTMModel()
    print("\nLSTM Model Architecture:")
    print(lstm_model.get_model_summary())
    
    engine = ColdChainMonitoringEngine(lstm_model)
    
    # Generate sample sensor data (15-hour history)
    print("\nGenerating synthetic sensor data (15 hours)...")
    base_time = datetime.now() - timedelta(hours=15)
    
    normal_readings = []
    for i in range(60):
        # Simulate slight warming trend
        temp_offset = 0.05 * i  # Gradual warming
        reading = SensorReading(
            sensor_id="SENSOR-01",
            equipment_id="FRIDGE-A",
            temperature=-3.5 + temp_offset + np.random.normal(0, 0.2),
            humidity=75.0,
            compressor_status="ON",
            door_status="CLOSED",
            ambient_temperature=25.0,
            timestamp=base_time + timedelta(minutes=15*i),
            is_anomaly=False
        )
        normal_readings.append(reading)
    
    # Process normal readings
    print("\nProcessing normal sensor readings...")
    for reading in normal_readings:
        alert = engine.process_sensor_reading(reading)
        if alert:
            print(f"  [{alert.severity.value}] {alert.alert_type.value}: {alert.message}")
    
    # Add critical reading (temperature violation)
    print("\nAdding critical temperature reading...")
    critical_reading = SensorReading(
        sensor_id="SENSOR-01",
        equipment_id="FRIDGE-A",
        temperature=1.5,  # Above safe range!
        humidity=80.0,
        compressor_status="OFF",  # Compressor off
        door_status="OPEN",
        ambient_temperature=28.0,
        timestamp=datetime.now(),
        is_anomaly=False
    )
    
    alert = engine.process_sensor_reading(critical_reading)
    if alert:
        print(f"\n[{alert.severity.value}] ALERT: {alert.alert_type.value}")
        print(f"  Message: {alert.message}")
        print(f"  Current Temp: {alert.current_temperature}°C")
        print(f"  Requires Immediate Action: {alert.requires_immediate_action}")
        if alert.remediation_actions:
            print(f"  Recommended Actions:")
            for action in alert.remediation_actions:
                print(f"    - {action}")
    
    # Equipment health assessment
    print("\n" + "="*80)
    print("EQUIPMENT HEALTH ASSESSMENT")
    print("="*80)
    
    health = engine.get_equipment_health_score("FRIDGE-A")
    print(f"\nEquipment ID: {health['equipment_id']}")
    print(f"Health Score: {health['health_score']}/100")
    print(f"Status: {health['status']}")
    if health['issues']:
        print(f"Issues:")
        for issue in health['issues']:
            print(f"  - {issue}")
    
    # Alert summary
    print("\n" + "="*80)
    print("ALERT SUMMARY")
    print("="*80)
    print(f"Total Alerts Generated: {len(engine.alerts)}")
    for alert in engine.alerts[-5:]:
        print(f"\n  {alert.alert_id}")
        print(f"    Type: {alert.alert_type.value}")
        print(f"    Severity: {alert.severity.value}")
        print(f"    Timestamp: {alert.timestamp}")


if __name__ == "__main__":
    example_cold_chain_monitoring()
