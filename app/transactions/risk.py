from decimal import Decimal

from app.db.models import Merchant


HIGH_RISK_CATEGORIES = {"crypto", "gambling", "wire_transfer"}
VELOCITY_REVIEW_THRESHOLD = 5
VELOCITY_WINDOW_SECONDS = 60


def evaluate_transaction(
    *,
    amount: Decimal,
    currency: str,
    merchant: Merchant,
    recent_transaction_count: int,
) -> tuple[str, int, str]:
    if amount >= Decimal("10000.00"):
        return "declined", 95, "Amount is above the automatic decline threshold"

    if amount >= Decimal("5000.00"):
        return "review", 75, "Amount is above the manual review threshold"

    if merchant.trust_status == "untrusted":
        return "review", 80, "Merchant is marked untrusted"

    if recent_transaction_count >= VELOCITY_REVIEW_THRESHOLD:
        return "review", 65, "Too many recent transactions for this user"

    if merchant.category.lower() in HIGH_RISK_CATEGORIES:
        return "review", 70, "Merchant category requires manual review"

    if currency.upper() != "USD":
        return "review", 60, "Non-USD transactions require manual review"

    return "approved", 20, "Transaction passed baseline risk checks"
