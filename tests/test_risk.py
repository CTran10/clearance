from decimal import Decimal

from app.db.models import Merchant
from app.transactions.risk import RiskRules, evaluate_transaction


def test_risk_rules_can_override_amount_thresholds():
    rules = RiskRules(
        review_amount_threshold=Decimal("100.00"),
        decline_amount_threshold=Decimal("200.00"),
        high_risk_categories=frozenset({"gift_card"}),
        velocity_review_threshold=3,
    )
    merchant = Merchant(category="retail", trust_status="trusted")

    review_decision = evaluate_transaction(
        amount=Decimal("150.00"),
        currency="USD",
        merchant=merchant,
        recent_transaction_count=0,
        risk_rules=rules,
    )
    decline_decision = evaluate_transaction(
        amount=Decimal("250.00"),
        currency="USD",
        merchant=merchant,
        recent_transaction_count=0,
        risk_rules=rules,
    )

    assert review_decision == (
        "review",
        75,
        "Amount is above the manual review threshold",
    )
    assert decline_decision == (
        "declined",
        95,
        "Amount is above the automatic decline threshold",
    )


def test_risk_rules_can_override_velocity_and_high_risk_categories():
    rules = RiskRules(
        review_amount_threshold=Decimal("5000.00"),
        decline_amount_threshold=Decimal("10000.00"),
        high_risk_categories=frozenset({"gift_card"}),
        velocity_review_threshold=2,
    )

    velocity_decision = evaluate_transaction(
        amount=Decimal("10.00"),
        currency="USD",
        merchant=Merchant(category="retail", trust_status="trusted"),
        recent_transaction_count=2,
        risk_rules=rules,
    )
    category_decision = evaluate_transaction(
        amount=Decimal("10.00"),
        currency="USD",
        merchant=Merchant(category="GIFT_CARD", trust_status="trusted"),
        recent_transaction_count=0,
        risk_rules=rules,
    )

    assert velocity_decision == (
        "review",
        65,
        "Too many recent transactions for this user",
    )
    assert category_decision == (
        "review",
        70,
        "Merchant category requires manual review",
    )
