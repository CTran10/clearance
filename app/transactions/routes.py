from fastapi import APIRouter, Depends, Header, HTTPException, Request, Response, status
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.db.dependencies import get_db
from app.db.models import Merchant, Transaction, User
from app.transactions.risk import evaluate_transaction
from app.transactions.schemas import (
    TransactionCreateRequest,
    TransactionListResponse,
    TransactionResponse,
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
    if not idempotency_key:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key header is required",
        )

    idempotency_key = idempotency_key.strip()

    if not idempotency_key:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key header is required",
        )

    if len(idempotency_key) > 255:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key must be 255 characters or fewer",
        )

    existing_transaction = (
        db.query(Transaction)
        .filter(
            Transaction.user_id == current_user.id,
            Transaction.idempotency_key == idempotency_key,
        )
        .first()
    )
    if existing_transaction:
        if (
            existing_transaction.merchant_id != payload.merchant_id
            or existing_transaction.amount != payload.amount
            or existing_transaction.currency != payload.currency
        ):
            raise HTTPException(
                status_code=status.HTTP_409_CONFLICT,
                detail="Idempotency-Key was already used with a different payload",
            )
        response.status_code = status.HTTP_200_OK
        return existing_transaction

    merchant = (
        db.query(Merchant)
        .filter(
            Merchant.id == payload.merchant_id,
            Merchant.owner_user_id == current_user.id,
        )
        .first()
    )
    if not merchant:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Merchant not found",
        )

    decision_status, risk_score, decision_reason = evaluate_transaction(
        amount=payload.amount,
        currency=payload.currency,
        merchant=merchant,
    )

    transaction = Transaction(
        user_id=current_user.id,
        merchant_id=merchant.id,
        amount=payload.amount,
        currency=payload.currency,
        status=decision_status,
        risk_score=risk_score,
        decision_reason=decision_reason,
        idempotency_key=idempotency_key,
    )
    db.add(transaction)

    try:
        db.flush()
        create_audit_event(
            db,
            action=f"TRANSACTION_{decision_status.upper()}",
            entity_type="transaction",
            entity_id=transaction.id,
            user_id=current_user.id,
            metadata={
                "merchant_id": merchant.id,
                "amount": str(payload.amount),
                "currency": payload.currency,
                "risk_score": risk_score,
                "decision_reason": decision_reason,
            },
            request_id=getattr(request.state, "request_id", None),
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        existing_transaction = (
            db.query(Transaction)
            .filter(
                Transaction.user_id == current_user.id,
                Transaction.idempotency_key == idempotency_key,
            )
            .first()
        )
        if existing_transaction:
            response.status_code = status.HTTP_200_OK
            return existing_transaction
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Transaction could not be created safely",
        )

    db.refresh(transaction)
    return transaction


@router.get("", response_model=TransactionListResponse)
def list_transactions(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    transactions = (
        db.query(Transaction)
        .filter(Transaction.user_id == current_user.id)
        .order_by(Transaction.created_at.desc())
        .all()
    )

    return {"transactions": transactions}


@router.get("/{transaction_id}", response_model=TransactionResponse)
def get_transaction(
    transaction_id: int,
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
):
    transaction = (
        db.query(Transaction)
        .filter(
            Transaction.id == transaction_id,
            Transaction.user_id == current_user.id,
        )
        .first()
    )
    if not transaction:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Transaction not found",
        )

    return transaction
