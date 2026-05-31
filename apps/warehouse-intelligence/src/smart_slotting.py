"""
Smart Slotting & Market Basket Analysis Module

Applies FP-Growth algorithm to historical order data to identify
frequently co-occurring items and optimize warehouse floor plan layout.

Author: Warehouse Intelligence System
Version: 1.0
"""

from dataclasses import dataclass
from typing import Dict, List, Set, Tuple, Optional
from collections import defaultdict, Counter
import logging
from datetime import datetime, timedelta

# For production: use mlxtend.frequent_patterns
# pip install mlxtend
try:
    from mlxtend.frequent_patterns import fpgrowth, association_rules
    import pandas as pd
    HAS_MLXTEND = True
except ImportError:
    HAS_MLXTEND = False
    logging.warning("mlxtend not installed. Using custom FP-Growth implementation.")


# ============================================================================
# Data Models
# ============================================================================

@dataclass
class Transaction:
    """Represents a single warehouse order transaction."""
    order_id: str
    items: List[str]  # Item IDs
    timestamp: datetime
    quantity_map: Dict[str, int]  # item_id -> quantity


@dataclass
class AssociationRule:
    """Represents a discovered association rule."""
    antecedent: frozenset  # Items that trigger the rule
    consequent: frozenset  # Items that are frequently co-picked
    support: float  # Percentage of transactions containing both
    confidence: float  # P(consequent | antecedent)
    lift: float  # Strength of association (1.0 = no correlation)
    leverage: float  # Absolute difference between observed and expected frequency


@dataclass
class SlottingRecommendation:
    """Warehouse slot assignment recommendation."""
    item_id: str
    recommended_rack: str
    reason: str  # Why this rack is optimal
    associated_items: List[str]  # Items frequently picked together
    confidence_score: float  # 0-1 confidence in recommendation
    placement_distance_meters: Optional[Dict[str, int]] = None  # Suggested distances to associated items


# ============================================================================
# Custom FP-Growth Implementation (Lightweight)
# ============================================================================

class FPNode:
    """Node in the FP-Tree structure."""
    
    def __init__(self, value: str, count: int, parent=None):
        self.value = value
        self.count = count
        self.parent = parent
        self.children: Dict[str, 'FPNode'] = {}
        self.next = None  # Link to next node with same value

    def increment(self, count: int):
        """Increase the count of this node."""
        self.count += count

    def display(self, indent: int = 0):
        """Pretty print the tree structure."""
        print(' ' * indent, self.value, ' ', self.count)
        for child in self.children.values():
            child.display(indent + 2)


class FPTree:
    """
    FP-Tree data structure for efficient frequent itemset mining.
    
    Pros: Eliminates multiple passes over data, more efficient than Apriori
    Cons: Complex implementation, higher memory overhead for small datasets
    """
    
    def __init__(self, transactions: List[List[str]], min_support: int, sort_order: List[str]):
        self.min_support = min_support
        self.sort_order = sort_order
        self.root = FPNode(None, 1, None)
        self.header_table: Dict[str, FPNode] = {}
        
        # Build the FP-Tree
        for transaction in transactions:
            sorted_items = [item for item in sort_order if item in transaction]
            if sorted_items:
                self._insert_transaction(sorted_items, self.root, 1)
    
    def _insert_transaction(self, items: List[str], node: FPNode, count: int):
        """Insert a transaction into the FP-Tree."""
        if not items:
            return
        
        first_item = items[0]
        
        if first_item in node.children:
            node.children[first_item].increment(count)
        else:
            new_node = FPNode(first_item, count, node)
            node.children[first_item] = new_node
            self._update_header_table(first_item, new_node)
        
        remaining_items = items[1:]
        if remaining_items:
            self._insert_transaction(remaining_items, node.children[first_item], count)
    
    def _update_header_table(self, item: str, node: FPNode):
        """Update the header table to track all nodes with same item."""
        if item in self.header_table:
            current = self.header_table[item]
            while current.next:
                current = current.next
            current.next = node
        else:
            self.header_table[item] = node


class SmartSlottingEngine:
    """
    Market Basket Analysis engine for warehouse slotting optimization.
    
    Workflow:
    1. Load historical order transactions
    2. Apply FP-Growth to mine frequent itemsets
    3. Generate association rules
    4. Convert rules to slotting recommendations
    5. Output rack assignments with inter-item distances
    """
    
    def __init__(self, min_support: float = 0.02, min_confidence: float = 0.3, 
                 min_lift: float = 1.5):
        """
        Initialize the Smart Slotting Engine.
        
        Args:
            min_support: Minimum support threshold (2% of all transactions)
            min_confidence: Minimum confidence for rules (30%)
            min_lift: Minimum lift to consider rule significant (1.5x)
        """
        self.min_support = min_support
        self.min_confidence = min_confidence
        self.min_lift = min_lift
        self.transactions: List[Transaction] = []
        self.association_rules: List[AssociationRule] = []
        self.itemset_frequencies: Dict[frozenset, float] = {}
        
        self.logger = logging.getLogger(__name__)
    
    def load_transactions(self, transactions: List[Transaction]):
        """Load historical order transactions from database."""
        self.transactions = transactions
        self.logger.info(f"Loaded {len(transactions)} transactions for analysis")
    
    def mine_frequent_itemsets(self) -> Dict[frozenset, int]:
        """
        Mine frequent itemsets using FP-Growth algorithm.
        
        Returns:
            Dictionary mapping itemset (frozenset) to support count
        """
        if not self.transactions:
            raise ValueError("No transactions loaded. Call load_transactions() first.")
        
        # Convert transactions to item lists
        transaction_list = [txn.items for txn in self.transactions]
        total_transactions = len(transaction_list)
        
        if HAS_MLXTEND:
            return self._mine_with_mlxtend(transaction_list)
        else:
            return self._mine_with_fpgrowth_custom(transaction_list, total_transactions)
    
    def _mine_with_mlxtend(self, transaction_list: List[List[str]]) -> Dict[frozenset, int]:
        """Mine using mlxtend library (production-grade)."""
        # Create transaction database as binary matrix
        from mlxtend.preprocessing import TransactionEncoder
        
        te = TransactionEncoder()
        te_ary = te.fit(transaction_list).transform(transaction_list)
        df = pd.DataFrame(te_ary, columns=te.columns_)
        
        # Apply FP-Growth
        itemsets = fpgrowth(df, min_support=self.min_support, use_colnames=True)
        
        # Convert to our format
        frequent_itemsets = {}
        for items, support_val in zip(itemsets['itemsets'], itemsets['support']):
            frequent_itemsets[items] = int(support_val * len(transaction_list))
        
        return frequent_itemsets
    
    def _mine_with_fpgrowth_custom(self, transaction_list: List[List[str]], 
                                   total_transactions: int) -> Dict[frozenset, int]:
        """Lightweight custom FP-Growth implementation."""
        min_support_count = int(self.min_support * total_transactions)
        
        # Count single items
        item_counts = Counter()
        for transaction in transaction_list:
            for item in transaction:
                item_counts[item] += 1
        
        # Filter by minimum support
        frequent_items = {item: count for item, count in item_counts.items() 
                         if count >= min_support_count}
        
        # Sort by frequency (descending)
        sorted_items = sorted(frequent_items.keys(), 
                            key=lambda x: frequent_items[x], reverse=True)
        
        # For simplicity, generate pairs and triplets
        frequent_itemsets = {}
        
        # Add single items
        for item, count in frequent_items.items():
            frequent_itemsets[frozenset([item])] = count
        
        # Generate k-itemsets
        for i, transaction in enumerate(transaction_list):
            filtered = [item for item in transaction if item in frequent_items]
            
            # Pairs
            for j, item1 in enumerate(filtered):
                for item2 in filtered[j+1:]:
                    pair = frozenset([item1, item2])
                    frequent_itemsets[pair] = frequent_itemsets.get(pair, 0) + 1
            
            # Triplets (if support is high)
            for j, item1 in enumerate(filtered):
                for k, item2 in enumerate(filtered[j+1:]):
                    for item3 in filtered[k+j+2:]:
                        triplet = frozenset([item1, item2, item3])
                        if triplet not in frequent_itemsets:
                            frequent_itemsets[triplet] = 0
                        frequent_itemsets[triplet] += 1
        
        # Filter by min support again
        return {itemset: count for itemset, count in frequent_itemsets.items() 
                if count >= min_support_count}
    
    def generate_association_rules(self, frequent_itemsets: Dict[frozenset, int]) -> List[AssociationRule]:
        """
        Generate association rules from frequent itemsets.
        
        Process:
        - For each frequent itemset with 2+ items
        - Generate all possible (antecedent → consequent) splits
        - Calculate confidence, lift, leverage
        - Filter by minimum thresholds
        
        Returns:
            List of AssociationRule objects
        """
        rules = []
        total_transactions = len(self.transactions)
        
        for itemset, support_count in frequent_itemsets.items():
            if len(itemset) < 2:
                continue
            
            itemset_support = support_count / total_transactions
            
            # Generate all possible antecedent-consequent splits
            items = list(itemset)
            for i in range(1, len(items)):
                for antecedent_items in self._combinations(items, i):
                    antecedent = frozenset(antecedent_items)
                    consequent = itemset - antecedent
                    
                    antecedent_count = frequent_itemsets.get(antecedent, 0)
                    consequent_count = frequent_itemsets.get(consequent, 0)
                    
                    if antecedent_count == 0 or consequent_count == 0:
                        continue
                    
                    antecedent_support = antecedent_count / total_transactions
                    consequent_support = consequent_count / total_transactions
                    
                    # Calculate metrics
                    confidence = itemset_support / antecedent_support
                    lift = itemset_support / (antecedent_support * consequent_support)
                    leverage = itemset_support - (antecedent_support * consequent_support)
                    
                    # Apply thresholds
                    if confidence >= self.min_confidence and lift >= self.min_lift:
                        rules.append(AssociationRule(
                            antecedent=antecedent,
                            consequent=consequent,
                            support=itemset_support,
                            confidence=confidence,
                            lift=lift,
                            leverage=leverage
                        ))
        
        self.association_rules = sorted(rules, key=lambda r: r.lift, reverse=True)
        self.logger.info(f"Generated {len(rules)} association rules")
        return self.association_rules
    
    def _combinations(self, items: List[str], r: int):
        """Generate all combinations of r items from items list."""
        from itertools import combinations
        return combinations(items, r)
    
    def generate_slotting_recommendations(self) -> List[SlottingRecommendation]:
        """
        Convert association rules into warehouse slotting recommendations.
        
        Logic:
        - Analyze rules where high confidence items are frequently co-picked
        - Recommend placing associated items within 10-20 meters
        - Assign to same warehouse zone/aisle
        - Consider item velocity (faster-moving items to more accessible slots)
        
        Returns:
            List of SlottingRecommendation objects
        """
        recommendations = []
        item_to_associated = defaultdict(list)
        
        # Build item association map
        for rule in self.association_rules:
            for consequent_item in rule.consequent:
                for antecedent_item in rule.antecedent:
                    item_to_associated[consequent_item].append({
                        'item': antecedent_item,
                        'confidence': rule.confidence,
                        'lift': rule.lift
                    })
        
        # Generate recommendations
        processed_items = set()
        for item_id in item_to_associated:
            if item_id in processed_items:
                continue
            
            associated = sorted(
                item_to_associated[item_id],
                key=lambda x: x['lift'],
                reverse=True
            )[:5]  # Top 5 associated items
            
            associated_items = [a['item'] for a in associated]
            confidence_score = associated[0]['confidence'] if associated else 0.0
            
            # Assign rack based on item frequency and association strength
            rack = self._assign_rack(item_id, associated_items, confidence_score)
            
            recommendations.append(SlottingRecommendation(
                item_id=item_id,
                recommended_rack=rack,
                reason=f"High co-picking frequency with {len(associated_items)} associated items",
                associated_items=associated_items,
                confidence_score=confidence_score,
                placement_distance_meters={item: 10 + (5 if i > 0 else 0) 
                                          for i, item in enumerate(associated_items)}
            ))
            processed_items.add(item_id)
        
        self.logger.info(f"Generated {len(recommendations)} slotting recommendations")
        return recommendations
    
    def _assign_rack(self, item_id: str, associated_items: List[str], 
                    confidence: float) -> str:
        """
        Heuristic to assign item to warehouse rack.
        
        Logic:
        - High confidence (>0.7) → Premium aisle (fast-moving)
        - Medium confidence (0.5-0.7) → Standard aisle
        - Low confidence (<0.5) → Secondary aisle
        """
        # In production: integrate with warehouse layout database
        if confidence > 0.7:
            return f"AISLE-A-{hash(item_id) % 10}"  # Premium aisle
        elif confidence > 0.5:
            return f"AISLE-B-{hash(item_id) % 15}"  # Standard aisle
        else:
            return f"AISLE-C-{hash(item_id) % 20}"  # Secondary aisle
    
    def run_full_pipeline(self, transactions: List[Transaction]) -> List[SlottingRecommendation]:
        """
        Execute complete smart slotting pipeline.
        
        Steps:
        1. Load transactions
        2. Mine frequent itemsets
        3. Generate association rules
        4. Convert to slotting recommendations
        
        Returns:
            Warehouse slotting recommendations
        """
        self.load_transactions(transactions)
        itemsets = self.mine_frequent_itemsets()
        self.generate_association_rules(itemsets)
        recommendations = self.generate_slotting_recommendations()
        return recommendations


# ============================================================================
# Example Usage
# ============================================================================

def example_smart_slotting():
    """Demonstration of the Smart Slotting Engine."""
    
    logging.basicConfig(level=logging.INFO)
    
    # Sample data: Fresh produce warehouse orders
    sample_transactions = [
        Transaction(
            order_id="ORD-001",
            items=["APPLE", "BANANA", "ORANGE"],
            timestamp=datetime.now(),
            quantity_map={"APPLE": 10, "BANANA": 15, "ORANGE": 8}
        ),
        Transaction(
            order_id="ORD-002",
            items=["APPLE", "CARROT", "TOMATO"],
            timestamp=datetime.now() - timedelta(days=1),
            quantity_map={"APPLE": 20, "CARROT": 5, "TOMATO": 12}
        ),
        Transaction(
            order_id="ORD-003",
            items=["BANANA", "ORANGE", "STRAWBERRY"],
            timestamp=datetime.now() - timedelta(days=2),
            quantity_map={"BANANA": 8, "ORANGE": 10, "STRAWBERRY": 6}
        ),
        Transaction(
            order_id="ORD-004",
            items=["APPLE", "BANANA", "CARROT"],
            timestamp=datetime.now() - timedelta(days=3),
            quantity_map={"APPLE": 15, "BANANA": 12, "CARROT": 3}
        ),
        # Repeat patterns to establish frequency
        Transaction(
            order_id="ORD-005",
            items=["APPLE", "BANANA"],
            timestamp=datetime.now() - timedelta(days=4),
            quantity_map={"APPLE": 25, "BANANA": 20}
        ),
    ]
    
    # Initialize engine
    engine = SmartSlottingEngine(min_support=0.20, min_confidence=0.4, min_lift=1.2)
    
    # Run full pipeline
    recommendations = engine.run_full_pipeline(sample_transactions)
    
    # Display results
    print("\n" + "="*80)
    print("SMART SLOTTING RECOMMENDATIONS")
    print("="*80 + "\n")
    
    for rec in recommendations[:5]:
        print(f"Item ID: {rec.item_id}")
        print(f"  Recommended Rack: {rec.recommended_rack}")
        print(f"  Associated Items: {', '.join(rec.associated_items)}")
        print(f"  Confidence Score: {rec.confidence_score:.2%}")
        print(f"  Reason: {rec.reason}")
        print()
    
    # Display association rules
    print("\n" + "="*80)
    print("ASSOCIATION RULES (Market Basket Analysis)")
    print("="*80 + "\n")
    
    for i, rule in enumerate(engine.association_rules[:5], 1):
        print(f"Rule {i}:")
        print(f"  {set(rule.antecedent)} → {set(rule.consequent)}")
        print(f"  Support: {rule.support:.2%} | Confidence: {rule.confidence:.2%} | Lift: {rule.lift:.2f}")
        print()


if __name__ == "__main__":
    example_smart_slotting()
