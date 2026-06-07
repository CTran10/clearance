import re
from datetime import datetime, timedelta, timezone

from fastapi import HTTPException, status
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.db.models import Merchant, Transaction, User
from app.transactions.risk import VELOCITY_WINDOW_SECONDS, evaluate_transaction
from app.transactions.schemas import TransactionCreateRequest

IDEMPOTENCY_KEY_PATTERN = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")


def create_transaction_with_idempotency(
    db: Session,
    *,
    user: User,
    payload: TransactionCreateRequest,
    idempotency_key: str | None,
    request_id: str | None,
) -> tuple[Transaction, bool]:
    idempotency_key = normalize_idempotency_key(idempotency_key)
    existing_transaction = find_transaction_by_idempotency_key(
        db,
        user_id=user.id,
        idempotency_key=idempotency_key,
    )

    if existing_transaction:
        ensure_idempotent_payload_matches(existing_transaction, payload)
        return existing_transaction, False

    merchant = get_owned_merchant_or_404(db, merchant_id=payload.merchant_id, user_id=user.id)
    recent_transaction_count = count_recent_transactions_for_user(
        db,
        user_id=user.id,
        window_seconds=VELOCITY_WINDOW_SECONDS,
    )
    transaction = build_transaction(
        user=user,
        merchant=merchant,
        payload=payload,
        idempotency_key=idempotency_key,
        recent_transaction_count=recent_transaction_count,
    )
    db.add(transaction)

    try:
        db.flush()
        record_transaction_audit_event(
            db,
            transaction=transaction,
            merchant=merchant,
            user=user,
            request_id=request_id,
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        existing_transaction = find_transaction_by_idempotency_key(
            db,
            user_id=user.id,
            idempotency_key=idempotency_key,
        )
        if existing_transaction:
            ensure_idempotent_payload_matches(existing_transaction, payload)
            return existing_transaction, False
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Transaction could not be created safely",
        )

    db.refresh(transaction)
    return transaction, True


def normalize_idempotency_key(idempotency_key: str | None) -> str:
    if not idempotency_key:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key header is required",
        )

    normalized_key = idempotency_key.strip()
    if not normalized_key:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key header is required",
        )

    if not IDEMPOTENCY_KEY_PATTERN.fullmatch(normalized_key):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="Idempotency-Key must be 1-128 safe characters",
        )

    return normalized_key


def find_transaction_by_idempotency_key(
    db: Session,
    *,
    user_id: int,
    idempotency_key: str,
) -> Transaction | None:
    return (
        db.query(Transaction)
        .filter(
            Transaction.user_id == user_id,
            Transaction.idempotency_key == idempotency_key,
        )
        .first()
    )


def ensure_idempotent_payload_matches(
    transaction: Transaction,
    payload: TransactionCreateRequest,
) -> None:
    if (
        transaction.merchant_id != payload.merchant_id
        or transaction.amount != payload.amount
        or transaction.currency != payload.currency
    ):
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Idempotency-Key was already used with a different payload",
        )


def get_owned_merchant_or_404(db: Session, *, merchant_id: int, user_id: int) -> Merchant:
    merchant = (
        db.query(Merchant)
        .filter(
            Merchant.id == merchant_id,
            Merchant.owner_user_id == user_id,
        )
        .first()
    )
    if not merchant:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Merchant not found",
        )
    return merchant


def build_transaction(
    *,
    user: User,
    merchant: Merchant,
    payload: TransactionCreateRequest,
    idempotency_key: str,
    recent_transaction_count: int,
) -> Transaction:
    decision_status, risk_score, decision_reason = evaluate_transaction(
        amount=payload.amount,
        currency=payload.currency,
        merchant=merchant,
        recent_transaction_count=recent_transaction_count,
    )

    return Transaction(
        user_id=user.id,
        merchant_id=merchant.id,
        amount=payload.amount,
        currency=payload.currency,
        status=decision_status,
        risk_score=risk_score,
        decision_reason=decision_reason,
        idempotency_key=idempotency_key,
    )


def count_recent_transactions_for_user(
    db: Session,
    *,
    user_id: int,
    window_seconds: int,
) -> int:
    cutoff = datetime.now(timezone.utc) - timedelta(seconds=window_seconds)
    return (
        db.query(func.count(Transaction.id))
        .filter(
            Transaction.user_id == user_id,
            Transaction.created_at >= cutoff,
        )
        .scalar()
        or 0
    )


def record_transaction_audit_event(
    db: Session,
    *,
    transaction: Transaction,
    merchant: Merchant,
    user: User,
    request_id: str | None,
) -> None:
    create_audit_event(
        db,
        action=f"TRANSACTION_{transaction.status.upper()}",
        entity_type="transaction",
        entity_id=transaction.id,
        user_id=user.id,
        metadata={
            "merchant_id": merchant.id,
            "merchant_trust_status": merchant.trust_status,
            "amount": str(transaction.amount),
            "currency": transaction.currency,
            "risk_score": transaction.risk_score,
            "decision_reason": transaction.decision_reason,
        },
        request_id=request_id,
    )


def list_transactions_for_user(db: Session, user: User, *, limit: int) -> list[Transaction]:
    return (
        db.query(Transaction)
        .filter(Transaction.user_id == user.id)
        .order_by(Transaction.created_at.desc())
        .limit(limit)
        .all()
    )


def get_transaction_for_user_or_404(
    db: Session,
    *,
    transaction_id: int,
    user: User,
) -> Transaction:
    transaction = (
        db.query(Transaction)
        .filter(
            Transaction.id == transaction_id,
            Transaction.user_id == user.id,
        )
        .first()
    )
    if not transaction:
        raise HTTPException(
            status_code=status.HTTP_404_NOT_FOUND,
            detail="Transaction not found",
        )
    return transaction
