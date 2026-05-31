"""
Warehouse Intelligence System Configuration

Environment-based configuration for different deployment scenarios.
"""

import os
from typing import Dict, Any
from dataclasses import dataclass
from enum import Enum


class Environment(Enum):
    """Deployment environment."""
    DEVELOPMENT = "development"
    STAGING = "staging"
    PRODUCTION = "production"


@dataclass
class SlottingConfig:
    """Smart Slotting Engine configuration."""
    min_support: float = 0.02  # 2% minimum support
    min_confidence: float = 0.30  # 30% confidence threshold
    min_lift: float = 1.5  # Minimum lift requirement
    max_itemset_size: int = 5  # Max items per itemset
    batch_processing_interval: int = 3600  # Run every 1 hour (seconds)


@dataclass
class HazardConfig:
    """Hazard Segregation Engine configuration."""
    pesticide_distance_meters: int = 5
    toxic_distance_meters: int = 8
    flammable_distance_meters: int = 10
    hazmat_distance_meters: int = 6
    emergency_exit_min_distance: int = 30
    audit_log_retention_days: int = 90
    enable_auto_remediation: bool = False


@dataclass
class ColdChainConfig:
    """Cold-Chain Monitoring configuration."""
    lstm_enabled: bool = True
    lstm_input_timesteps: int = 60  # 15 hours (15-min intervals)
    lstm_prediction_horizon: int = 5  # Predict next 5 timesteps
    optimal_temp_min: float = -4.0
    optimal_temp_max: float = -2.0
    acceptable_temp_min: float = -22.0
    acceptable_temp_max: float = 0.0
    critical_temp_min: float = -25.0
    critical_temp_max: float = 2.0
    alert_rate_of_change_threshold: float = 1.0  # °C/hour
    lstm_batch_size: int = 32
    lstm_epochs: int = 100


@dataclass
class DatabaseConfig:
    """Database configuration."""
    host: str
    port: int
    database: str
    username: str
    password: str
    pool_size: int = 10
    max_overflow: int = 20


@dataclass
class APIConfig:
    """API configuration."""
    host: str = "0.0.0.0"
    port: int = 8000
    workers: int = 4
    timeout: int = 60
    max_request_size: int = 10_000_000  # 10MB
    cors_origins: list = None


@dataclass
class LoggingConfig:
    """Logging configuration."""
    level: str = "INFO"
    format: str = "%(asctime)s - %(name)s - %(levelname)s - %(message)s"
    file_path: str = "warehouse_intelligence.log"
    max_size_mb: int = 100
    backup_count: int = 5


class WarehouseIntelligenceConfig:
    """Main configuration class for Warehouse Intelligence System."""
    
    def __init__(self, environment: str = None):
        """
        Initialize configuration.
        
        Args:
            environment: "development", "staging", or "production"
                        Falls back to WAREHOUSE_ENV environment variable
        """
        env_str = environment or os.getenv("WAREHOUSE_ENV", "development")
        self.environment = Environment(env_str)
        
        # Load environment-specific settings
        self._load_config()
    
    def _load_config(self):
        """Load configuration based on environment."""
        if self.environment == Environment.DEVELOPMENT:
            self._load_development_config()
        elif self.environment == Environment.STAGING:
            self._load_staging_config()
        elif self.environment == Environment.PRODUCTION:
            self._load_production_config()
    
    def _load_development_config(self):
        """Development environment configuration."""
        self.slotting = SlottingConfig(
            min_support=0.05,  # More permissive for testing
            min_confidence=0.25,
            batch_processing_interval=300  # 5 minutes
        )
        
        self.hazard = HazardConfig(
            enable_auto_remediation=False
        )
        
        self.coldchain = ColdChainConfig(
            lstm_enabled=False  # Faster development without LSTM
        )
        
        self.database = DatabaseConfig(
            host="localhost",
            port=5432,
            database="warehouse_dev",
            username="postgres",
            password="postgres"
        )
        
        self.api = APIConfig(
            host="0.0.0.0",
            port=8000,
            workers=1
        )
        
        self.logging = LoggingConfig(
            level="DEBUG",
            file_path="logs/warehouse_intelligence_dev.log"
        )
    
    def _load_staging_config(self):
        """Staging environment configuration."""
        self.slotting = SlottingConfig(
            min_support=0.02,
            min_confidence=0.30,
            batch_processing_interval=1800  # 30 minutes
        )
        
        self.hazard = HazardConfig(
            enable_auto_remediation=False,
            audit_log_retention_days=30
        )
        
        self.coldchain = ColdChainConfig(
            lstm_enabled=True,
            lstm_batch_size=32
        )
        
        self.database = DatabaseConfig(
            host=os.getenv("DB_HOST", "staging-db.internal"),
            port=int(os.getenv("DB_PORT", "5432")),
            database=os.getenv("DB_NAME", "warehouse_staging"),
            username=os.getenv("DB_USER", "warehouse"),
            password=os.getenv("DB_PASSWORD", "")
        )
        
        self.api = APIConfig(
            host="0.0.0.0",
            port=8000,
            workers=4
        )
        
        self.logging = LoggingConfig(
            level="INFO",
            file_path="logs/warehouse_intelligence_staging.log"
        )
    
    def _load_production_config(self):
        """Production environment configuration."""
        self.slotting = SlottingConfig(
            min_support=0.02,
            min_confidence=0.30,
            batch_processing_interval=3600  # 1 hour
        )
        
        self.hazard = HazardConfig(
            enable_auto_remediation=True,
            audit_log_retention_days=365  # 1 year
        )
        
        self.coldchain = ColdChainConfig(
            lstm_enabled=True,
            lstm_batch_size=64,
            lstm_epochs=150
        )
        
        self.database = DatabaseConfig(
            host=os.getenv("DB_HOST", "prod-db.internal"),
            port=int(os.getenv("DB_PORT", "5432")),
            database=os.getenv("DB_NAME", "warehouse_prod"),
            username=os.getenv("DB_USER", "warehouse"),
            password=os.getenv("DB_PASSWORD", ""),
            pool_size=20,
            max_overflow=40
        )
        
        self.api = APIConfig(
            host="0.0.0.0",
            port=8000,
            workers=8,
            timeout=120
        )
        
        self.logging = LoggingConfig(
            level="INFO",
            file_path="logs/warehouse_intelligence_production.log",
            backup_count=30
        )
    
    def to_dict(self) -> Dict[str, Any]:
        """Export configuration as dictionary."""
        return {
            "environment": self.environment.value,
            "slotting": self.slotting.__dict__,
            "hazard": self.hazard.__dict__,
            "coldchain": self.coldchain.__dict__,
            "database": {
                "host": self.database.host,
                "port": self.database.port,
                "database": self.database.database,
                "pool_size": self.database.pool_size
            },
            "api": self.api.__dict__,
            "logging": self.logging.__dict__
        }


# ============================================================================
# Configuration Singletons
# ============================================================================

_CONFIG = None


def get_config() -> WarehouseIntelligenceConfig:
    """Get global configuration instance."""
    global _CONFIG
    if _CONFIG is None:
        _CONFIG = WarehouseIntelligenceConfig()
    return _CONFIG


def load_config(environment: str = None) -> WarehouseIntelligenceConfig:
    """Load and set global configuration."""
    global _CONFIG
    _CONFIG = WarehouseIntelligenceConfig(environment)
    return _CONFIG


# ============================================================================
# Example Usage
# ============================================================================

if __name__ == "__main__":
    import json
    
    # Load configuration for current environment
    config = load_config("development")
    
    # Print configuration
    print("Warehouse Intelligence Configuration")
    print("=" * 80)
    print(json.dumps(config.to_dict(), indent=2))
    
    # Access specific settings
    print("\nSlotting Configuration:")
    print(f"  Min Support: {config.slotting.min_support}")
    print(f"  Min Confidence: {config.slotting.min_confidence}")
    print(f"  Min Lift: {config.slotting.min_lift}")
    
    print("\nHazard Segregation Configuration:")
    print(f"  Pesticide Distance: {config.hazard.pesticide_distance_meters}m")
    print(f"  Toxic Distance: {config.hazard.toxic_distance_meters}m")
    print(f"  Flammable Distance: {config.hazard.flammable_distance_meters}m")
    
    print("\nCold-Chain Configuration:")
    print(f"  LSTM Enabled: {config.coldchain.lstm_enabled}")
    print(f"  Optimal Temp Range: {config.coldchain.optimal_temp_min}°C to {config.coldchain.optimal_temp_max}°C")
    print(f"  Critical Temp Range: {config.coldchain.critical_temp_min}°C to {config.coldchain.critical_temp_max}°C")
