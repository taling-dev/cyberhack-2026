"""
Unit Tests for Warehouse Intelligence System

Tests for all three core modules with example scenarios.
"""

import pytest
from datetime import datetime, timedelta
import numpy as np

from src.smart_slotting import (
    SmartSlottingEngine, Transaction, AssociationRule
)
from src.hazard_segregation import (
    HazardSegregationEngine, Item, StorageLocation, AdjacencyInfo,
    HazardLevel, StorageCategory, ComplianceStatus
)
from src.cold_chain_monitoring import (
    ColdChainMonitoringEngine, SensorReading, AlertSeverity
)


# ============================================================================
# Smart Slotting Tests
# ============================================================================

class TestSmartSlotting:
    """Test suite for Smart Slotting & Market Basket Analysis."""
    
    @pytest.fixture
    def engine(self):
        """Initialize Smart Slotting Engine."""
        return SmartSlottingEngine(
            min_support=0.15,
            min_confidence=0.4,
            min_lift=1.2
        )
    
    @pytest.fixture
    def sample_transactions(self):
        """Generate sample fresh produce transactions."""
        return [
            Transaction("ORD-001", ["APPLE", "BANANA"], datetime.now(), {"APPLE": 10, "BANANA": 15}),
            Transaction("ORD-002", ["APPLE", "CARROT"], datetime.now(), {"APPLE": 20, "CARROT": 5}),
            Transaction("ORD-003", ["BANANA", "ORANGE"], datetime.now(), {"BANANA": 8, "ORANGE": 10}),
            Transaction("ORD-004", ["APPLE", "BANANA"], datetime.now(), {"APPLE": 15, "BANANA": 12}),
            Transaction("ORD-005", ["APPLE", "BANANA", "CARROT"], datetime.now(), {"APPLE": 25, "BANANA": 20, "CARROT": 3}),
            Transaction("ORD-006", ["STRAWBERRY", "ORANGE"], datetime.now(), {"STRAWBERRY": 6, "ORANGE": 12}),
            Transaction("ORD-007", ["APPLE", "BANANA"], datetime.now(), {"APPLE": 18, "BANANA": 14}),
        ]
    
    def test_engine_initialization(self, engine):
        """Test engine initialization with correct parameters."""
        assert engine.min_support == 0.15
        assert engine.min_confidence == 0.4
        assert engine.min_lift == 1.2
    
    def test_load_transactions(self, engine, sample_transactions):
        """Test loading transactions into engine."""
        engine.load_transactions(sample_transactions)
        assert len(engine.transactions) == len(sample_transactions)
    
    def test_mine_frequent_itemsets(self, engine, sample_transactions):
        """Test frequent itemset mining."""
        engine.load_transactions(sample_transactions)
        itemsets = engine.mine_frequent_itemsets()
        
        assert len(itemsets) > 0
        # Apple should be frequent (appears in most orders)
        apple_itemset = frozenset(["APPLE"])
        assert apple_itemset in itemsets
    
    def test_generate_association_rules(self, engine, sample_transactions):
        """Test association rule generation."""
        engine.load_transactions(sample_transactions)
        itemsets = engine.mine_frequent_itemsets()
        rules = engine.generate_association_rules(itemsets)
        
        assert len(rules) > 0
        for rule in rules:
            assert rule.confidence >= engine.min_confidence
            assert rule.lift >= engine.min_lift
    
    def test_slotting_recommendations(self, engine, sample_transactions):
        """Test generation of warehouse slotting recommendations."""
        recommendations = engine.run_full_pipeline(sample_transactions)
        
        assert len(recommendations) > 0
        for rec in recommendations:
            assert rec.item_id is not None
            assert rec.recommended_rack is not None
            assert 0 <= rec.confidence_score <= 1.0


# ============================================================================
# Hazard Segregation Tests
# ============================================================================

class TestHazardSegregation:
    """Test suite for Hazard Segregation compliance engine."""
    
    @pytest.fixture
    def engine(self):
        """Initialize Hazard Segregation Engine."""
        return HazardSegregationEngine()
    
    def test_pesticide_safe_placement(self, engine):
        """Test pesticide in proper hazmat zone (should PASS)."""
        pesticide = Item(
            item_id="PST-001",
            name="Glyphosate",
            hazard_level=HazardLevel.PESTICIDE,
            compliance_standard="INI"
        )
        
        location = StorageLocation(
            rack_id="HAZMAT-01",
            category=StorageCategory.PESTICIDE_STORAGE,
            zone="H",
            temperature=20.0,
            ventilation_level="HIGH"
        )
        
        result = engine.validate_item_placement(pesticide, location, [])
        
        assert result.is_approved
        assert result.status != ComplianceStatus.REJECTED
    
    def test_pesticide_near_food(self, engine):
        """Test pesticide near fresh produce (should FAIL)."""
        pesticide = Item(
            item_id="PST-001",
            name="Pesticide",
            hazard_level=HazardLevel.PESTICIDE,
            compliance_standard="INI"
        )
        
        location = StorageLocation(
            rack_id="HAZMAT-01",
            category=StorageCategory.PESTICIDE_STORAGE,
            zone="H"
        )
        
        adjacent_food = AdjacencyInfo(
            rack_id="PRODUCE-01",
            distance_meters=3,  # Too close!
            category=StorageCategory.FRESH_PRODUCE,
            items=[Item("APPLE-01", "Apples", HazardLevel.SAFE, "FDA")]
        )
        
        result = engine.validate_item_placement(pesticide, location, [adjacent_food])
        
        assert not result.is_approved
        assert result.status == ComplianceStatus.REJECTED
        assert len(result.violations) > 0
    
    def test_refrigerated_temperature_check(self, engine):
        """Test refrigerated item with correct temperature (should PASS)."""
        refrigerated = Item(
            item_id="REF-001",
            name="Fresh Berries",
            hazard_level=HazardLevel.REFRIGERATED,
            compliance_standard="FDA"
        )
        
        cold_location = StorageLocation(
            rack_id="COLD-01",
            category=StorageCategory.REFRIGERATED,
            zone="C",
            temperature=-3.0  # Within spec (-4 to 4°C)
        )
        
        result = engine.validate_item_placement(refrigerated, cold_location, [])
        
        assert result.is_approved
    
    def test_refrigerated_temperature_violation(self, engine):
        """Test refrigerated item with wrong temperature (should FAIL)."""
        refrigerated = Item(
            item_id="REF-001",
            name="Berries",
            hazard_level=HazardLevel.REFRIGERATED,
            compliance_standard="FDA"
        )
        
        warm_location = StorageLocation(
            rack_id="COLD-01",
            category=StorageCategory.REFRIGERATED,
            zone="C",
            temperature=15.0  # Way too warm!
        )
        
        result = engine.validate_item_placement(refrigerated, warm_location, [])
        
        assert not result.is_approved
        assert result.status == ComplianceStatus.REJECTED
    
    def test_audit_trail(self, engine):
        """Test audit trail recording."""
        item = Item("ITEM-01", "Test Item", HazardLevel.SAFE, "FDA")
        location = StorageLocation("RACK-01", StorageCategory.GENERAL, "A")
        
        engine.validate_item_placement(item, location, [])
        
        audit_trail = engine.get_audit_trail("ITEM-01")
        assert len(audit_trail) == 1


# ============================================================================
# Cold-Chain Monitoring Tests
# ============================================================================

class TestColdChainMonitoring:
    """Test suite for Cold-Chain Monitoring with LSTM."""
    
    @pytest.fixture
    def engine(self):
        """Initialize Cold-Chain Monitoring Engine."""
        return ColdChainMonitoringEngine()
    
    def test_normal_temperature_reading(self, engine):
        """Test normal temperature reading (no alert)."""
        reading = SensorReading(
            sensor_id="SENSOR-01",
            equipment_id="FRIDGE-A",
            temperature=-3.5,
            humidity=75.0,
            compressor_status="ON",
            timestamp=datetime.now()
        )
        
        alert = engine.process_sensor_reading(reading)
        
        # Should not trigger alert for normal conditions
        assert alert is None or alert.severity != AlertSeverity.CRITICAL
    
    def test_critical_temperature_alert(self, engine):
        """Test critical temperature violation (should alert)."""
        reading = SensorReading(
            sensor_id="SENSOR-01",
            equipment_id="FRIDGE-A",
            temperature=5.0,  # Above critical max (2.0)!
            humidity=80.0,
            compressor_status="OFF",
            timestamp=datetime.now()
        )
        
        alert = engine.process_sensor_reading(reading)
        
        assert alert is not None
        assert alert.severity == AlertSeverity.CRITICAL
    
    def test_equipment_health_score(self, engine):
        """Test equipment health assessment."""
        base_time = datetime.now() - timedelta(hours=1)
        
        # Add multiple stable readings
        for i in range(10):
            reading = SensorReading(
                sensor_id="SENSOR-01",
                equipment_id="FRIDGE-A",
                temperature=-3.5 + np.random.normal(0, 0.1),
                humidity=75.0,
                compressor_status="ON",
                timestamp=base_time + timedelta(minutes=i*10)
            )
            engine.process_sensor_reading(reading)
        
        health = engine.get_equipment_health_score("FRIDGE-A")
        
        assert "health_score" in health
        assert 0 <= health["health_score"] <= 100
        assert health["status"] in ["CRITICAL", "WARNING", "HEALTHY"]
    
    def test_sensor_validation(self, engine):
        """Test invalid sensor reading detection."""
        # Unrealistic temperature (too high)
        reading = SensorReading(
            sensor_id="SENSOR-01",
            equipment_id="FRIDGE-A",
            temperature=100.0,  # Unrealistic!
            timestamp=datetime.now()
        )
        
        alert = engine.process_sensor_reading(reading)
        
        # Should detect sensor failure
        assert alert is not None


# ============================================================================
# Integration Tests
# ============================================================================

class TestSystemIntegration:
    """Integration tests across all modules."""
    
    def test_complete_workflow(self):
        """Test complete warehouse intelligence workflow."""
        from src import WarehouseIntelligenceSystem
        
        system = WarehouseIntelligenceSystem(config={
            'min_support': 0.15,
            'min_confidence': 0.4,
            'use_lstm': False  # Skip LSTM for faster tests
        })
        
        # Get system status
        status = system.get_system_status()
        assert status is not None
        assert "modules" in status


# ============================================================================
# Performance Tests
# ============================================================================

class TestPerformance:
    """Performance benchmarking tests."""
    
    def test_hazard_segregation_latency(self):
        """Verify hazard segregation meets <100ms requirement."""
        import time
        
        engine = HazardSegregationEngine()
        
        item = Item("ITEM-01", "Test", HazardLevel.PESTICIDE, "INI")
        location = StorageLocation("RACK-01", StorageCategory.PESTICIDE_STORAGE, "H")
        
        start = time.time()
        for _ in range(100):
            engine.validate_item_placement(item, location, [])
        elapsed = time.time() - start
        
        avg_latency = (elapsed / 100) * 1000  # Convert to ms
        assert avg_latency < 100, f"Average latency {avg_latency}ms exceeds 100ms"


# ============================================================================
# Run Tests
# ============================================================================

if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])


class TestDrumSegregation:
    """SimaOps drum/hazard-class segregation rule."""

    def test_drum_placement_rules(self):
        e = HazardSegregationEngine()
        # No hazard -> always approved.
        assert e.validate_drum_placement("HAZARD_CLASS_NONE", ["IBC"], []).is_approved
        # Compatible drum + empty whitelist -> approved.
        assert e.validate_drum_placement("HAZARD_CLASS_IBC", ["IBC"], []).is_approved
        # Slot can't hold the drum -> rejected.
        assert not e.validate_drum_placement("HAZARD_CLASS_IBC", ["IPPC"], []).is_approved
        # Zone whitelist excludes the hazard class -> rejected.
        assert not e.validate_drum_placement("HAZARD_CLASS_IBC", ["IBC"], ["IPPC"]).is_approved


class TestPerZoneThresholds:
    """Per-equipment (zone-type) temperature thresholds."""

    def test_ambient_zone_rejects_freezing_temp(self):
        e = ColdChainMonitoringEngine()
        # Ambient zone: 15–25 °C acceptable, critical outside 10–30.
        e.equipment_thresholds["A"] = {
            "OPTIMAL_MIN": 15, "OPTIMAL_MAX": 25, "ACCEPTABLE_MIN": 13,
            "ACCEPTABLE_MAX": 27, "CRITICAL_MIN": 10, "CRITICAL_MAX": 30,
        }
        ok = e.process_sensor_reading(SensorReading(sensor_id="s-A", equipment_id="A", temperature=20.0, timestamp=datetime.now()))
        assert ok is None  # 20°C is fine for ambient
        bad = e.process_sensor_reading(SensorReading(sensor_id="s-A", equipment_id="A", temperature=-3.0, timestamp=datetime.now()))
        assert bad is not None and bad.severity == AlertSeverity.CRITICAL  # freezing in ambient = critical
