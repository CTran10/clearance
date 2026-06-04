from decimal import Decimal

from app.db.models import Merchant


HIGH_RISK_CATEGORIES = {"crypto", "gambling", "wire_transfer"}


def evaluate_transaction(
    *,
    amount: Decimal,
    currency: str,
    merchant: Merchant,
) -> tuple[str, int, str]:  # returning a 3-tuple felt clean until i forgot which slot was which. (status, score, reason). future me pls make this a dataclass
    # ORDER MATTERS here and i learned that by breaking it. the $10k decline check HAS to be first —
    # if i put the "review" checks first, a 50 thousand dollar crypto txn would just get a chill "review" and slide through.
    # first match wins, so most-severe goes on top
    if amount >= Decimal("10000.00"):
        return "declined", 95, "Amount is above the automatic decline threshold"

    if amount >= Decimal("5000.00"):
        return "review", 75, "Amount is above the manual review threshold"

    if merchant.category.lower() in HIGH_RISK_CATEGORIES:
        return "review", 70, "Merchant category requires manual review"

    if currency.upper() != "USD":
        return "review", 60, "Non-USD transactions require manual review"

    return "approved", 20, "Transaction passed baseline risk checks"
