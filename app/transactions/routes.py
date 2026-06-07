from fastapi import APIRouter, Depends, Header, Query, Request, Response, status
from sqlalchemy.orm import Session

from app.core.request_context import get_request_id
from app.db.dependencies import get_db
from app.db.models import User
from app.transactions.schemas import (
    TransactionCreateRequest,
    TransactionListResponse,
    TransactionResponse,
)
from app.transactions.service import (
    create_transaction_with_idempotency,
    get_transaction_for_user_or_404,
    list_transactions_for_user,
)
from app.users.dependencies import get_current_user

router = APIRouter(prefix="/transactions", tags=["transactions"])


@router.post(
    "",
    status_code=status.HTTP_201_CREATED,
    response_model=TransactionResponse,
)
def create_transaction(
    payload: TransactionCreateRequest,
    request: Request,
    response: Response,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
    idempotency_key: str | None = Header(default=None, alias="Idempotency-Key"),
):
    transaction, created = create_transaction_with_idempotency(
        db,
        user=current_user,
        payload=payload,
        idempotency_key=idempotency_key,
        request_id=get_request_id(request),
    )
    if not created:
        response.status_code = status.HTTP_200_OK

    return transaction


@router.get("", response_model=TransactionListResponse)
def list_transactions(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
    limit: int = Query(default=100, ge=1, le=500),
):
    return {"transactions": list_transactions_for_user(db, current_user, limit=limit)}


@router.get("/{transaction_id}", response_model=TransactionResponse)
def get_transaction(
    transaction_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    return get_transaction_for_user_or_404(
        db,
        transaction_id=transaction_id,
        user=current_user,
    )
