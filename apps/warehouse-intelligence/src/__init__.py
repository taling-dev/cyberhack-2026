"""
Warehouse Intelligence System - Main Integration Module

Provides unified interface for all three warehouse operation modules:
1. Smart Slotting (ML-based)
2. Hazard Segregation (Rule-based)
3. Cold-Chain Monitoring (LSTM-based)

Integration with warehouse API endpoints.
"""

import logging
from typing import Dict, List, Optional
from datetime import datetime

from .smart_slotting import SmartSlottingEngine, Transaction
from .hazard_segregation import HazardSegregationEngine, Item, StorageLocation, AdjacencyInfo
from .cold_chain_monitoring import ColdChainMonitoringEngine, SensorReading, ColdChainLSTMModel


# ============================================================================
# Warehouse Intelligence System
# ============================================================================

class WarehouseIntelligenceSystem:
    """
    Unified warehouse intelligence platform integrating all three modules.
    
    Typical workflow:
    1. Smart Slotting: Analyze historical orders → optimize layout
    2. Hazard Segregation: Validate item placements → ensure compliance
    3. Cold-Chain: Monitor temperatures → predictive maintenance
    """
    
    def __init__(self, config: Optional[Dict] = None):
        """
        Initialize Warehouse Intelligence System.
        
        Args:
            config: Configuration dict with module parameters
        """
        self.config = config or {}
        self.logger = logging.getLogger(__name__)
        
        # Initialize modules
        self.slotting_engine = SmartSlottingEngine(
            min_support=self.config.get('min_support', 0.02),
            min_confidence=self.config.get('min_confidence', 0.3),
            min_lift=self.config.get('min_lift', 1.5)
        )
        
        self.hazard_engine = HazardSegregationEngine()
        
        lstm_model = ColdChainLSTMModel() if self.config.get('use_lstm', True) else None
        self.coldchain_engine = ColdChainMonitoringEngine(lstm_model)
        
        self.logger.info("Warehouse Intelligence System initialized")
    
    def optimize_slotting(self, transactions: List[Transaction]) -> Dict:
        """
        Execute smart slotting optimization pipeline.
        
        Returns:
            {
                "recommendations": List[SlottingRecommendation],
                "association_rules": List[AssociationRule],
                "metrics": {
                    "itemsets_discovered": int,
                    "rules_generated": int,
                    "coverage": float
                }
            }
        """
        self.logger.info(f"Starting slotting optimization for {len(transactions)} transactions")
        
        recommendations = self.slotting_engine.run_full_pipeline(transactions)
        
        return {
            "status": "SUCCESS",
            "recommendations": recommendations,
            "association_rules": self.slotting_engine.association_rules,
            "metrics": {
                "transactions_analyzed": len(transactions),
                "itemsets_discovered": len(self.slotting_engine.itemset_frequencies),
                "rules_generated": len(self.slotting_engine.association_rules),
            }
        }
    
    def validate_placement(
        self,
        item: Item,
        location: StorageLocation,
        adjacent_locations: List[AdjacencyInfo]
    ) -> Dict:
        """
        Validate hazardous material placement.
        
        Returns:
            {
                "status": ComplianceStatus.value,
                "is_approved": bool,
                "audit_report": Dict,
                "remediation": List[str]
            }
        """
        self.logger.info(f"Validating placement for item {item.item_id}")
        
        result = self.hazard_engine.validate_item_placement(
            item, location, adjacent_locations
        )
        
        return {
            "status": result.status.value,
            "is_approved": result.is_approved,
            "audit_report": self.hazard_engine.export_compliance_report(result),
            "remediation": result.remediation_actions
        }
    
    def process_sensor_data(self, reading: SensorReading) -> Optional[Dict]:
        """
        Process cold-chain sensor reading.
        
        Returns:
            {
                "alert": {
                    "alert_id": str,
                    "severity": str,
                    "message": str,
                    "actions": List[str]
                } if alert triggered,
                "equipment_health": Dict
            }
        """
        alert = self.coldchain_engine.process_sensor_reading(reading)
        
        return {
            "equipment_id": reading.equipment_id,
            "current_temperature": reading.temperature,
            "alert": {
                "alert_id": alert.alert_id,
                "severity": alert.severity.value,
                "type": alert.alert_type.value,
                "message": alert.message,
                "actions": alert.remediation_actions,
                "requires_immediate_action": alert.requires_immediate_action
            } if alert else None,
            "equipment_health": self.coldchain_engine.get_equipment_health_score(reading.equipment_id)
        }
    
    def get_system_status(self) -> Dict:
        """Get overall system status across all modules."""
        return {
            "timestamp": datetime.now().isoformat(),
            "modules": {
                "slotting": {
                    "status": "READY",
                    "transactions_loaded": len(self.slotting_engine.transactions),
                    "rules_generated": len(self.slotting_engine.association_rules)
                },
                "hazard_segregation": {
                    "status": "READY",
                    "validations_performed": len(self.hazard_engine.validation_history)
                },
                "cold_chain": {
                    "status": "READY",
                    "sensors_tracked": len(self.coldchain_engine.sensor_history),
                    "active_alerts": len(self.coldchain_engine.alerts),
                    "critical_alerts": sum(1 for a in self.coldchain_engine.alerts 
                                          if a.requires_immediate_action)
                }
            }
        }


# ============================================================================
# API Integration Layer
# ============================================================================

class WarehouseIntelligenceAPI:
    """
    API endpoints for Warehouse Intelligence System.
    
    Integration with apps/api backend:
    - POST /api/warehouse/slotting/optimize
    - POST /api/warehouse/hazard/validate
    - POST /api/warehouse/coldchain/process
    - GET /api/warehouse/system/status
    """
    
    def __init__(self, system: WarehouseIntelligenceSystem):
        self.system = system
        self.logger = logging.getLogger(__name__)
    
    async def optimize_slotting_endpoint(self, payload: Dict) -> Dict:
        """
        API endpoint: POST /api/warehouse/slotting/optimize
        
        Payload:
            {
                "transactions": [{
                    "order_id": "ORD-001",
                    "items": ["APPLE", "BANANA"],
                    "timestamp": "2026-05-31T10:00:00Z",
                    "quantity_map": {"APPLE": 10, "BANANA": 15}
                }]
            }
        """
        try:
            transactions = [
                Transaction(
                    order_id=t["order_id"],
                    items=t["items"],
                    timestamp=t.get("timestamp", datetime.now()),
                    quantity_map=t.get("quantity_map", {})
                )
                for t in payload.get("transactions", [])
            ]
            
            result = self.system.optimize_slotting(transactions)
            return {"success": True, "data": result}
        
        except Exception as e:
            self.logger.error(f"Slotting optimization failed: {str(e)}")
            return {"success": False, "error": str(e)}
    
    async def validate_placement_endpoint(self, payload: Dict) -> Dict:
        """
        API endpoint: POST /api/warehouse/hazard/validate
        
        Payload:
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
        """
        try:
            # Parse payload and validate
            result = self.system.validate_placement(
                # Implement payload parsing
            )
            return {"success": True, "data": result}
        
        except Exception as e:
            self.logger.error(f"Placement validation failed: {str(e)}")
            return {"success": False, "error": str(e)}
    
    async def process_sensor_endpoint(self, payload: Dict) -> Dict:
        """
        API endpoint: POST /api/warehouse/coldchain/process
        
        Payload:
            {
                "sensor_id": "SENSOR-01",
                "equipment_id": "FRIDGE-A",
                "temperature": -3.5,
                "timestamp": "2026-05-31T10:00:00Z"
            }
        """
        try:
            reading = SensorReading(
                sensor_id=payload["sensor_id"],
                equipment_id=payload["equipment_id"],
                temperature=float(payload["temperature"]),
                timestamp=payload.get("timestamp", datetime.now())
            )
            
            result = self.system.process_sensor_data(reading)
            return {"success": True, "data": result}
        
        except Exception as e:
            self.logger.error(f"Sensor processing failed: {str(e)}")
            return {"success": False, "error": str(e)}


# ============================================================================
# Example Integration
# ============================================================================

def example_system_integration():
    """Demonstrate full system integration."""
    
    logging.basicConfig(level=logging.INFO)
    
    # Initialize system
    system = WarehouseIntelligenceSystem(
        config={
            'min_support': 0.15,
            'min_confidence': 0.4,
            'use_lstm': True
        }
    )
    
    # Get system status
    status = system.get_system_status()
    print("\nSystem Status:")
    print(status)


if __name__ == "__main__":
    example_system_integration()
