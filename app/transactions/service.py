import hashlib
import json
import re
from datetime import UTC, datetime, timedelta
from decimal import Decimal

from fastapi import HTTPException, status
from sqlalchemy import func
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.core.config import settings
from app.db.models import Merchant, Transaction, User
from app.transactions.risk import build_risk_rules, evaluate_transaction
from app.transactions.schemas import TransactionCreateRequest

IDEMPOTENCY_KEY_PATTERN = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")
MONEY_QUANTIZER = Decimal("0.01")


def create_transaction_with_idempotency(
    db: Session,
    *,
    user: User,
    payload: TransactionCreateRequest,
    idempotency_key: str | None,
    request_id: str | None,
) -> tuple[Transaction, bool]:
    idempotency_key = normalize_idempotency_key(idempotency_key)
    idempotency_request_hash = build_idempotency_request_hash(payload)
    existing_transaction = find_transaction_by_idempotency_key(
        db,
        user_id=user.id,
        idempotency_key=idempotency_key,
    )

    if existing_transaction:
        ensure_idempotent_payload_matches(
            existing_transaction,
            payload,
            idempotency_request_hash=idempotency_request_hash,
        )
        return existing_transaction, False

    merchant = get_owned_merchant_or_404(db, merchant_id=payload.merchant_id, user_id=user.id)
    recent_transaction_count = count_recent_transactions_for_user(
        db,
        user_id=user.id,
        window_seconds=settings.risk_velocity_window_seconds,
    )
    transaction = build_transaction(
        user=user,
        merchant=merchant,
        payload=payload,
        idempotency_key=idempotency_key,
        idempotency_request_hash=idempotency_request_hash,
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
            ensure_idempotent_payload_matches(
                existing_transaction,
                payload,
                idempotency_request_hash=idempotency_request_hash,
            )
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


def build_idempotency_request_hash(payload: TransactionCreateRequest) -> str:
    # whole point: fingerprint the request so if someone reuses an idempotency key with a DIFFERENT body
    # (oops, or sketchy) we can notice. but a naive json hash is a trap — {"a":1,"b":2} and {"b":2,"a":1}
    # are the same request but hash differently! so i force a "canonical" form first:
    canonical_payload = {
        "amount": format(payload.amount.quantize(MONEY_QUANTIZER), "f"),  # "5" and "5.00" must hash the same → quantize to cents
        "currency": payload.currency.upper(),                              # "usd" == "USD"
        "merchant_id": payload.merchant_id,
    }
    encoded_payload = json.dumps(
        canonical_payload,
        separators=(",", ":"),  # kill the spaces json.dumps loves to add, otherwise "a: 1" vs "a:1" → different hash
        sort_keys=True,         # always same key order. THIS is the line that took me an hour to figure out
    ).encode("utf-8")
    return hashlib.sha256(encoded_payload).hexdigest()


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
    *,
    idempotency_request_hash: str | None = None,
) -> None:
    request_hash = idempotency_request_hash or build_idempotency_request_hash(payload)
    # the hash path is the new hotness. the field-by-field check below is the legacy fallback for old rows
    # that got saved before i added the hash column — didn't want to wipe them, so i keep both paths alive
    if transaction.idempotency_request_hash:
        if transaction.idempotency_request_hash != request_hash:
            # same key, different request = someone's confused (or malicious). hard no, don't silently reuse the old result
            raise_idempotency_conflict()
        return

    if (
        transaction.merchant_id != payload.merchant_id
        or transaction.amount != payload.amount
        or transaction.currency != payload.currency
    ):
        raise_idempotency_conflict()


def raise_idempotency_conflict() -> None:
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
    idempotency_request_hash: str,
    recent_transaction_count: int,
) -> Transaction:
    decision_status, risk_score, decision_reason = evaluate_transaction(
        amount=payload.amount,
        currency=payload.currency,
        merchant=merchant,
        recent_transaction_count=recent_transaction_count,
        risk_rules=build_risk_rules(settings),
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
        idempotency_request_hash=idempotency_request_hash,
    )


def count_recent_transactions_for_user(
    db: Session,
    *,
    user_id: int,
    window_seconds: int,
) -> int:
    cutoff = datetime.now(UTC) - timedelta(seconds=window_seconds)
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
