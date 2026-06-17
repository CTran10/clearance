from dataclasses import dataclass
from decimal import Decimal

from app.core.config import Settings, settings
from app.db.models import Merchant


# frozen=True = nobody can accidentally reassign a threshold at runtime. risk rules shouldn't mutate mid-request.
# learned this after a different bug where shared mutable config got stomped by one request and leaked into the next 💀
@dataclass(frozen=True)
class RiskRules:
    review_amount_threshold: Decimal
    decline_amount_threshold: Decimal
    high_risk_categories: frozenset[str]
    velocity_review_threshold: int


def build_risk_rules(app_settings: Settings = settings) -> RiskRules:
    return RiskRules(
        review_amount_threshold=app_settings.risk_review_amount_threshold,
        decline_amount_threshold=app_settings.risk_decline_amount_threshold,
        high_risk_categories=frozenset(
            category.lower() for category in app_settings.risk_high_risk_categories
        ),
        velocity_review_threshold=app_settings.risk_velocity_review_threshold,
    )


def evaluate_transaction(
    *,
    amount: Decimal,
    currency: str,
    merchant: Merchant,
    recent_transaction_count: int,
    risk_rules: RiskRules | None = None,
) -> tuple[str, int, str]:  # returning a 3-tuple felt clean until i forgot which slot was which. (status, score, reason). future me pls make this a dataclass
    # finally yanked the magic numbers out into RiskRules so ops can tune thresholds via env without a redeploy.
    active_rules = risk_rules or build_risk_rules()

    # ORDER STILL MATTERS and i learned that by breaking it. the decline check HAS to be first —
    # if i put the "review" checks first, a giant crypto txn would just get a chill "review" and slide through.
    # first match wins, so most-severe goes on top
    if amount >= active_rules.decline_amount_threshold:
        return "declined", 95, "Amount is above the automatic decline threshold"

    if amount >= active_rules.review_amount_threshold:
        return "review", 75, "Amount is above the manual review threshold"

    if merchant.trust_status == "untrusted":
        return "review", 80, "Merchant is marked untrusted"

    if recent_transaction_count >= active_rules.velocity_review_threshold:
        return "review", 65, "Too many recent transactions for this user"

    if merchant.category.lower() in active_rules.high_risk_categories:
        return "review", 70, "Merchant category requires manual review"

    if currency.upper() != "USD":
        return "review", 60, "Non-USD transactions require manual review"

    return "approved", 20, "Transaction passed baseline risk checks"
