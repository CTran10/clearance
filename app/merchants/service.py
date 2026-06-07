from sqlalchemy.orm import Session

from app.db.models import Merchant, User
from app.merchants.schemas import MerchantCreateRequest


def create_merchant_for_user(
    db: Session,
    *,
    owner: User,
    payload: MerchantCreateRequest,
) -> Merchant:
    merchant = Merchant(
        owner_user_id=owner.id,
        name=payload.name,
        category=payload.category,
        trust_status=payload.trust_status,
    )
    db.add(merchant)
    db.flush()
    return merchant


def list_merchants_for_user(db: Session, owner: User, *, limit: int) -> list[Merchant]:
    return (
        db.query(Merchant)
        .filter(Merchant.owner_user_id == owner.id)
        .order_by(Merchant.created_at.desc())
        .limit(limit)
        .all()
    )
