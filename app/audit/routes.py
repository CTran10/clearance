from datetime import UTC, datetime

from fastapi import APIRouter, Depends, HTTPException, Query, status
from sqlalchemy.orm import Session

from app.audit.schemas import AuditEventListResponse
from app.db.dependencies import get_db
from app.db.models import AuditEvent, User
from app.users.dependencies import get_current_user

router = APIRouter(prefix="/audit-events", tags=["audit-events"])


@router.get("", response_model=AuditEventListResponse)
def list_audit_events(
    current_user: User = Depends(get_current_user),
    db: Session = Depends(get_db),
    limit: int = Query(default=100, ge=1, le=500),
    action: str | None = Query(default=None, min_length=1, max_length=100),
    entity_type: str | None = Query(default=None, min_length=1, max_length=100),
    entity_id: int | None = Query(default=None, ge=1),
    created_after: datetime | None = None,
    created_before: datetime | None = None,
):
    created_after = normalize_filter_datetime(created_after)
    created_before = normalize_filter_datetime(created_before)

    if created_after and created_before and created_after >= created_before:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="created_after must be before created_before",
        )

    query = db.query(AuditEvent).filter(AuditEvent.user_id == current_user.id)

    if action:
        query = query.filter(AuditEvent.action == action)
    if entity_type:
        query = query.filter(AuditEvent.entity_type == entity_type)
    if entity_id:
        query = query.filter(AuditEvent.entity_id == entity_id)
    if created_after:
        query = query.filter(AuditEvent.created_at >= created_after)
    if created_before:
        query = query.filter(AuditEvent.created_at < created_before)

    audit_events = query.order_by(AuditEvent.created_at.desc()).limit(limit).all()

    return {"audit_events": audit_events}


def normalize_filter_datetime(value: datetime | None) -> datetime | None:
    if value is None:
        return None
    if value.tzinfo is None:
        return value
    return value.astimezone(UTC).replace(tzinfo=None)
