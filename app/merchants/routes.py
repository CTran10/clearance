from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.db.dependencies import get_db
from app.db.models import Merchant, User
from app.merchants.schemas import (
    MerchantCreateRequest,
    MerchantListResponse,
    MerchantResponse,
)
from app.users.dependencies import get_current_user

router = APIRouter(prefix="/merchants", tags=["merchants"])


@router.post(
    "",
    status_code=status.HTTP_201_CREATED,
    response_model=MerchantResponse,
)
def create_merchant(
    payload: MerchantCreateRequest,
    request: Request,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    merchant = Merchant(
        owner_user_id=current_user.id,
        name=payload.name,
        category=payload.category,
    )

    db.add(merchant)
    db.flush()
    create_audit_event(
        db,
        action="CREATED_MERCHANT",
        entity_type="merchant",
        entity_id=merchant.id,
        user_id=current_user.id,
        request_id=getattr(request.state, "request_id", None),
        metadata={"name": merchant.name, "category": merchant.category},
    )
    db.commit()
    db.refresh(merchant)

    return merchant


@router.get("", response_model=MerchantListResponse)
def list_merchants(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    merchants = (
        db.query(Merchant)
        .filter(Merchant.owner_user_id == current_user.id)
        .all()
    )

    return {"merchants": merchants}
