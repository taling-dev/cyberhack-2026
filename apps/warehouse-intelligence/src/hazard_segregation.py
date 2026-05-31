"""
Hazard Segregation Module - Rule-Based Compliance Engine

Implements deterministic rule-based validation for hazardous material
segregation in fresh produce warehouses. NO machine learning.

Ensures compliance with:
- INI (Ikatan Nasional Indonesia) regulations
- IBC (Intermediate Bulk Container) standards
- FDA guidelines for food storage
- OSHA hazmat segregation requirements

Author: Warehouse Intelligence System
Version: 1.0
"""

from dataclasses import dataclass, field
from typing import Dict, List, Tuple, Optional
from enum import Enum
from datetime import datetime
import uuid
import logging
import json


# ============================================================================
# Enums & Constants
# ============================================================================

class HazardLevel(Enum):
    """Classification of material hazard levels."""
    SAFE = "SAFE"
    REFRIGERATED = "REFRIGERATED"  # Requires controlled temperature
    HAZMAT = "HAZMAT"  # Hazardous material
    PESTICIDE = "PESTICIDE"  # Pesticide/herbicide
    TOXIC = "TOXIC"  # Toxic/corrosive substance
    FLAMMABLE = "FLAMMABLE"  # Fire hazard


class StorageCategory(Enum):
    """Warehouse storage area categories."""
    FRESH_PRODUCE = "FRESH_PRODUCE"
    REFRIGERATED = "REFRIGERATED"
    BEVERAGE = "BEVERAGE"
    HAZMAT_ZONE = "HAZMAT_ZONE"
    PESTICIDE_STORAGE = "PESTICIDE_STORAGE"
    FLAMMABLE_STORAGE = "FLAMMABLE_STORAGE"
    GENERAL = "GENERAL"


class ComplianceStatus(Enum):
    """Result of compliance validation."""
    APPROVED = "APPROVED"
    REJECTED = "REJECTED"
    NEEDS_REVIEW = "NEEDS_REVIEW"
    CONDITIONAL = "CONDITIONAL"  # Requires additional actions


# ============================================================================
# Segregation Rules (INI/IBC Standards)
# ============================================================================

HAZMAT_SEGREGATION_DISTANCES = {
    # Minimum meters between hazmat and food items
    HazardLevel.PESTICIDE: 5,  # 5m minimum
    HazardLevel.TOXIC: 8,  # 8m minimum (corrosives/acids)
    HazardLevel.FLAMMABLE: 10,  # 10m minimum (fire hazard)
    HazardLevel.HAZMAT: 6,
}

PROHIBITED_ADJACENCIES = {
    # HazardLevel: [prohibited_categories_nearby]
    HazardLevel.PESTICIDE: [StorageCategory.FRESH_PRODUCE, StorageCategory.BEVERAGE],
    HazardLevel.TOXIC: [StorageCategory.FRESH_PRODUCE, StorageCategory.BEVERAGE, StorageCategory.REFRIGERATED],
    HazardLevel.FLAMMABLE: [StorageCategory.HAZMAT_ZONE, StorageCategory.PESTICIDE_STORAGE],
    HazardLevel.REFRIGERATED: [StorageCategory.FLAMMABLE_STORAGE],
}

TEMPERATURE_REQUIREMENTS = {
    HazardLevel.REFRIGERATED: {"min": -4, "max": 4, "unit": "°C"},
    HazardLevel.PESTICIDE: {"min": 10, "max": 25, "unit": "°C"},
    HazardLevel.FLAMMABLE: {"min": 15, "max": 25, "unit": "°C"},
}

VENTILATION_REQUIREMENTS = {
    HazardLevel.PESTICIDE: "HIGH",  # 6+ air changes/hour
    HazardLevel.TOXIC: "VERY_HIGH",  # 10+ air changes/hour
    HazardLevel.FLAMMABLE: "HIGH",
}


# ============================================================================
# Data Models
# ============================================================================

@dataclass
class Item:
    """Represents a warehouse item."""
    item_id: str
    name: str
    hazard_level: HazardLevel
    compliance_standard: str  # e.g., "INI", "FDA", "IBC"
    quantity: int = 1
    batch_number: Optional[str] = None
    expiry_date: Optional[datetime] = None
    requires_cold_chain: bool = False


@dataclass
class StorageLocation:
    """Represents a warehouse storage slot/rack."""
    rack_id: str
    category: StorageCategory
    zone: str  # Zone identifier (A, B, C, etc.)
    current_items: List[Item] = field(default_factory=list)
    temperature: float = 20.0  # Celsius
    ventilation_level: str = "STANDARD"
    distance_to_emergency_exit: int = 50  # meters


@dataclass
class AdjacencyInfo:
    """Information about adjacent storage locations."""
    rack_id: str
    distance_meters: int
    category: StorageCategory
    items: List[Item]


@dataclass
class ComplianceResult:
    """Result of hazard segregation validation."""
    status: ComplianceStatus
    is_approved: bool
    violations: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    recommendations: List[str] = field(default_factory=list)
    audit_id: str = field(default_factory=lambda: str(uuid.uuid4()))
    timestamp: datetime = field(default_factory=datetime.now)
    checked_rules: List[str] = field(default_factory=list)  # For traceability
    remediation_actions: List[str] = field(default_factory=list)


# ============================================================================
# Hazard Segregation Engine
# ============================================================================

class HazardSegregationEngine:
    """
    Deterministic rule-based engine for hazard segregation validation.
    
    Features:
    - Fast execution (< 100ms per validation)
    - Fully auditable (all decisions traced)
    - No machine learning (reproducible, compliant)
    - Decision matrix approach for clarity
    
    Rule Categories:
    1. Hazard Classification Rules
    2. Distance & Segregation Rules
    3. Temperature & Environmental Rules
    4. Emergency Access Rules
    5. Zone-Based Rules
    """
    
    def __init__(self):
        self.logger = logging.getLogger(__name__)
        self.validation_history: List[Tuple[str, ComplianceResult]] = []
    
    def validate_drum_placement(
        self, hazard_class: str, drum_compatibility: List[str], hazard_allowed: List[str]
    ) -> ComplianceResult:
        """SimaOps drum/hazard-class segregation rule (single source of truth).

        hazard_class is the proto enum string ("HAZARD_CLASS_IBC"); the location
        arrays hold bare drum codes ("IBC"). Rules:
          - drum_compatibility: PHYSICAL constraint — slot must hold this drum.
          - hazard_allowed: SEGREGATION whitelist — when non-empty, the zone
            only accepts the listed hazard classes (empty = no restriction).
        """
        result = ComplianceResult(status=ComplianceStatus.APPROVED, is_approved=True)
        result.checked_rules.append("SIMAOPS_DRUM_001")

        drum = (hazard_class or "").replace("HAZARD_CLASS_", "")
        if drum and drum != "NONE":
            if drum not in drum_compatibility:
                result.violations.append(f"slot cannot hold {drum} drum")
            elif hazard_allowed and drum not in hazard_allowed:
                result.violations.append(f"zone segregation rejects hazard class {drum}")

        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
        self.validation_history.append((drum or "NONE", result))
        return result
    
    def validate_item_placement(
        self,
        item: Item,
        proposed_location: StorageLocation,
        adjacent_locations: List[AdjacencyInfo]
    ) -> ComplianceResult:
        """
        Validate whether an item can be safely stored at proposed location.
        
        Decision Flow:
        1. Check item hazard classification
        2. Verify temperature requirements
        3. Check adjacent items for prohibited combinations
        4. Validate segregation distances
        5. Verify emergency access
        
        Args:
            item: Item to be placed
            proposed_location: Target storage location
            adjacent_locations: Nearby storage locations with items
        
        Returns:
            ComplianceResult with approval status and detailed reasoning
        """
        result = ComplianceResult(
            status=ComplianceStatus.APPROVED,
            is_approved=True,
        )
        
        # Rule 1: Item Classification Check
        self._check_item_classification(item, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 2: Location Category Compatibility
        self._check_location_compatibility(item, proposed_location, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 3: Temperature Requirements
        self._check_temperature_requirements(item, proposed_location, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 4: Adjacent Items Segregation
        self._check_adjacent_segregation(item, adjacent_locations, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 5: Segregation Distance
        self._check_segregation_distances(item, adjacent_locations, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 6: Ventilation Requirements
        self._check_ventilation_requirements(item, proposed_location, result)
        if result.violations:
            result.status = ComplianceStatus.REJECTED
            result.is_approved = False
            return result
        
        # Rule 7: Emergency Access
        self._check_emergency_access(item, proposed_location, result)
        if result.warnings:  # This is a warning, not rejection
            result.status = ComplianceStatus.CONDITIONAL if not result.violations else result.status
        
        # Finalize result
        result.is_approved = result.status in [ComplianceStatus.APPROVED, ComplianceStatus.CONDITIONAL]
        self.validation_history.append((item.item_id, result))
        
        self.logger.info(f"Validation for {item.item_id}: {result.status.value} "
                        f"(Rules checked: {len(result.checked_rules)})")
        
        return result
    
    def _check_item_classification(self, item: Item, result: ComplianceResult):
        """Rule 1: Verify item hazard classification is valid."""
        rule_id = "ITEM_CLASS_001"
        result.checked_rules.append(rule_id)
        
        try:
            HazardLevel[item.hazard_level.name]  # Validate enum
            result.recommendations.append(
                f"Item classified as {item.hazard_level.value}"
            )
        except KeyError:
            result.violations.append(
                f"[{rule_id}] Invalid hazard classification: {item.hazard_level}"
            )
    
    def _check_location_compatibility(self, item: Item, location: StorageLocation, result: ComplianceResult):
        """Rule 2: Verify item category matches location category."""
        rule_id = "LOC_COMPAT_002"
        result.checked_rules.append(rule_id)
        
        # Decision Matrix: Item Hazard Level → Location Category
        compatibility_matrix = {
            HazardLevel.SAFE: [StorageCategory.GENERAL, StorageCategory.FRESH_PRODUCE],
            HazardLevel.REFRIGERATED: [StorageCategory.REFRIGERATED],
            HazardLevel.PESTICIDE: [StorageCategory.PESTICIDE_STORAGE, StorageCategory.HAZMAT_ZONE],
            HazardLevel.TOXIC: [StorageCategory.HAZMAT_ZONE],
            HazardLevel.FLAMMABLE: [StorageCategory.FLAMMABLE_STORAGE, StorageCategory.HAZMAT_ZONE],
            HazardLevel.HAZMAT: [StorageCategory.HAZMAT_ZONE],
        }
        
        allowed_categories = compatibility_matrix.get(item.hazard_level, [])
        
        if location.category not in allowed_categories:
            result.violations.append(
                f"[{rule_id}] {item.hazard_level.value} items cannot be stored in "
                f"{location.category.value} zone. Allowed: {[c.value for c in allowed_categories]}"
            )
        else:
            result.recommendations.append(
                f"[{rule_id}] Location category {location.category.value} is compatible"
            )
    
    def _check_temperature_requirements(self, item: Item, location: StorageLocation, result: ComplianceResult):
        """Rule 3: Verify temperature conditions meet item requirements."""
        rule_id = "TEMP_REQ_003"
        result.checked_rules.append(rule_id)
        
        if item.hazard_level not in TEMPERATURE_REQUIREMENTS:
            return  # No specific temperature requirement
        
        temp_spec = TEMPERATURE_REQUIREMENTS[item.hazard_level]
        min_temp = temp_spec["min"]
        max_temp = temp_spec["max"]
        
        if not (min_temp <= location.temperature <= max_temp):
            result.violations.append(
                f"[{rule_id}] Temperature {location.temperature}°C out of range "
                f"({min_temp}°C to {max_temp}°C) for {item.hazard_level.value}"
            )
        else:
            result.recommendations.append(
                f"[{rule_id}] Temperature requirement satisfied: {location.temperature}°C"
            )
    
    def _check_adjacent_segregation(self, item: Item, adjacent_locs: List[AdjacencyInfo], result: ComplianceResult):
        """Rule 4: Verify no prohibited items are stored adjacently."""
        rule_id = "ADJ_SEG_004"
        result.checked_rules.append(rule_id)
        
        prohibited_categories = PROHIBITED_ADJACENCIES.get(item.hazard_level, [])
        
        if not prohibited_categories:
            return  # No restrictions
        
        violations_found = []
        for adj_loc in adjacent_locs:
            if adj_loc.category in prohibited_categories:
                for adj_item in adj_loc.items:
                    violations_found.append(
                        f"{item.name} (HAZMAT) cannot be adjacent to {adj_item.name} "
                        f"({adj_loc.category.value})"
                    )
        
        if violations_found:
            result.violations.append(
                f"[{rule_id}] Prohibited adjacency violations: {'; '.join(violations_found)}"
            )
        else:
            result.recommendations.append(
                f"[{rule_id}] No prohibited adjacent items detected"
            )
    
    def _check_segregation_distances(self, item: Item, adjacent_locs: List[AdjacencyInfo], result: ComplianceResult):
        """Rule 5: Verify minimum segregation distances are maintained."""
        rule_id = "SEG_DIST_005"
        result.checked_rules.append(rule_id)
        
        if item.hazard_level not in HAZMAT_SEGREGATION_DISTANCES:
            return  # No distance requirement
        
        min_distance = HAZMAT_SEGREGATION_DISTANCES[item.hazard_level]
        
        for adj_loc in adjacent_locs:
            # Check if adjacent location has food/beverage items
            if adj_loc.category in [StorageCategory.FRESH_PRODUCE, StorageCategory.BEVERAGE]:
                if adj_loc.distance_meters < min_distance:
                    result.violations.append(
                        f"[{rule_id}] Distance to {adj_loc.category.value} "
                        f"({adj_loc.distance_meters}m) less than minimum ({min_distance}m)"
                    )
                    result.remediation_actions.append(
                        f"Move hazmat item to location at least {min_distance}m away from food items"
                    )
    
    def _check_ventilation_requirements(self, item: Item, location: StorageLocation, result: ComplianceResult):
        """Rule 6: Verify ventilation is adequate."""
        rule_id = "VENT_REQ_006"
        result.checked_rules.append(rule_id)
        
        if item.hazard_level not in VENTILATION_REQUIREMENTS:
            return  # No ventilation requirement
        
        required_level = VENTILATION_REQUIREMENTS[item.hazard_level]
        required_priority = ["STANDARD", "HIGH", "VERY_HIGH"]
        location_priority = required_priority.index(location.ventilation_level)
        required_priority_idx = required_priority.index(required_level)
        
        if location_priority < required_priority_idx:
            result.warnings.append(
                f"[{rule_id}] Ventilation level {location.ventilation_level} "
                f"may be insufficient for {item.hazard_level.value} "
                f"(recommended: {required_level})"
            )
            result.remediation_actions.append(
                f"Upgrade ventilation to {required_level} or relocate item"
            )
    
    def _check_emergency_access(self, item: Item, location: StorageLocation, result: ComplianceResult):
        """Rule 7: Verify emergency exit is accessible."""
        rule_id = "EMERG_ACC_007"
        result.checked_rules.append(rule_id)
        
        # Hazmat items should not block emergency routes
        if item.hazard_level in [HazardLevel.FLAMMABLE, HazardLevel.TOXIC]:
            if location.distance_to_emergency_exit < 30:  # Less than 30m
                result.warnings.append(
                    f"[{rule_id}] {item.hazard_level.value} item stored {location.distance_to_emergency_exit}m "
                    f"from emergency exit (recommended: >30m)"
                )
                result.remediation_actions.append(
                    "Relocate hazmat to position further from emergency routes"
                )
    
    def get_audit_trail(self, item_id: str) -> List[ComplianceResult]:
        """Retrieve audit trail for an item."""
        return [result for iid, result in self.validation_history if iid == item_id]
    
    def export_compliance_report(self, result: ComplianceResult) -> Dict:
        """Export compliance result as structured report."""
        return {
            "audit_id": result.audit_id,
            "timestamp": result.timestamp.isoformat(),
            "status": result.status.value,
            "is_approved": result.is_approved,
            "violations": result.violations,
            "warnings": result.warnings,
            "recommendations": result.recommendations,
            "rules_checked": result.checked_rules,
            "remediation_actions": result.remediation_actions,
        }


# ============================================================================
# Example Usage
# ============================================================================

def example_hazard_segregation():
    """Demonstration of Hazard Segregation Engine."""
    
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s'
    )
    
    engine = HazardSegregationEngine()
    
    print("\n" + "="*80)
    print("HAZARD SEGREGATION VALIDATION SYSTEM")
    print("="*80 + "\n")
    
    # Test Case 1: Pesticide near fresh produce (SHOULD FAIL)
    print("TEST CASE 1: Pesticide Storage near Fresh Produce")
    print("-" * 80)
    
    pesticide = Item(
        item_id="PST-001",
        name="Glyphosate Herbicide",
        hazard_level=HazardLevel.PESTICIDE,
        compliance_standard="INI"
    )
    
    pesticide_location = StorageLocation(
        rack_id="HAZMAT-05",
        category=StorageCategory.PESTICIDE_STORAGE,
        zone="H",
        temperature=20.0,
        ventilation_level="HIGH"
    )
    
    adjacent_fresh = AdjacencyInfo(
        rack_id="PRODUCE-01",
        distance_meters=3,  # Too close!
        category=StorageCategory.FRESH_PRODUCE,
        items=[Item("APPLE-001", "Apples", HazardLevel.SAFE, "FDA")]
    )
    
    result1 = engine.validate_item_placement(
        pesticide, pesticide_location, [adjacent_fresh]
    )
    
    print(f"Status: {result1.status.value}")
    print(f"Approved: {result1.is_approved}")
    print(f"Violations:")
    for v in result1.violations:
        print(f"  - {v}")
    print(f"Remediation:")
    for r in result1.remediation_actions:
        print(f"  - {r}")
    print()
    
    # Test Case 2: Refrigerated item with correct temperature (SHOULD PASS)
    print("\nTEST CASE 2: Refrigerated Item in Proper Conditions")
    print("-" * 80)
    
    refrigerated = Item(
        item_id="REF-001",
        name="Fresh Berries",
        hazard_level=HazardLevel.REFRIGERATED,
        compliance_standard="FDA",
        requires_cold_chain=True
    )
    
    cold_storage = StorageLocation(
        rack_id="COLD-10",
        category=StorageCategory.REFRIGERATED,
        zone="C",
        temperature=-2.0,  # Within spec (-4 to 4°C)
        ventilation_level="STANDARD"
    )
    
    result2 = engine.validate_item_placement(
        refrigerated, cold_storage, []
    )
    
    print(f"Status: {result2.status.value}")
    print(f"Approved: {result2.is_approved}")
    print(f"Recommendations:")
    for r in result2.recommendations:
        print(f"  ✓ {r}")
    print()
    
    # Test Case 3: Flammable item too close to emergency exit (WARNING)
    print("\nTEST CASE 3: Flammable Item Near Emergency Exit")
    print("-" * 80)
    
    flammable = Item(
        item_id="FLAM-001",
        name="Packaging Material (Foam)",
        hazard_level=HazardLevel.FLAMMABLE,
        compliance_standard="IBC"
    )
    
    near_exit = StorageLocation(
        rack_id="GEN-20",
        category=StorageCategory.FLAMMABLE_STORAGE,
        zone="F",
        temperature=18.0,
        ventilation_level="HIGH",
        distance_to_emergency_exit=25  # Close to exit
    )
    
    result3 = engine.validate_item_placement(
        flammable, near_exit, []
    )
    
    print(f"Status: {result3.status.value}")
    print(f"Approved: {result3.is_approved}")
    if result3.warnings:
        print(f"Warnings:")
        for w in result3.warnings:
            print(f"  ⚠ {w}")
    print()
    
    # Audit Trail Report
    print("\nAUDIT REPORT - Compliance Summary")
    print("-" * 80)
    
    for i, result in enumerate([result1, result2, result3], 1):
        report = engine.export_compliance_report(result)
        print(f"\nAudit ID: {report['audit_id']}")
        print(f"Timestamp: {report['timestamp']}")
        print(f"Status: {report['status']}")
        print(f"Rules Evaluated: {len(report['rules_checked'])}")


if __name__ == "__main__":
    example_hazard_segregation()
