from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.core.request_context import get_request_id
from app.db.dependencies import get_db
from app.db.models import User
from app.merchants.schemas import (
    MerchantCreateRequest,
    MerchantListResponse,
    MerchantResponse,
)
from app.merchants.service import create_merchant_for_user, list_merchants_for_user
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
    merchant = create_merchant_for_user(db, owner=current_user, payload=payload)
    create_audit_event(
        db,
        action="CREATED_MERCHANT",
        entity_type="merchant",
        entity_id=merchant.id,
        user_id=current_user.id,
        request_id=get_request_id(request),
        metadata={"name": merchant.name, "category": merchant.category},
    )
    db.commit()
    db.refresh(merchant)

    return merchant


@router.get("", response_model=MerchantListResponse)
def list_merchants(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
    limit: int = Query(default=100, ge=1, le=500),
):
    return {"merchants": list_merchants_for_user(db, current_user, limit=limit)}
