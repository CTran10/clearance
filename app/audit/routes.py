from fastapi import APIRouter, Depends, Query
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
):
    audit_events = (
        db.query(AuditEvent)
        .filter(AuditEvent.user_id == current_user.id)
        .order_by(AuditEvent.created_at.desc())
        .limit(limit)
        .all()
    )

    return {"audit_events": audit_events}
