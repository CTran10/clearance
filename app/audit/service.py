import json
from typing import Any

from sqlalchemy.orm import Session

from app.db.models import AuditEvent


def create_audit_event(
    db: Session,
    *,
    action: str,
    entity_type: str,
    user_id: int | None = None,
    entity_id: int | None = None,
    request_id: str | None = None,
    metadata: dict[str, Any] | None = None,
) -> AuditEvent:
    event = AuditEvent(
        user_id=user_id,
        action=action,
        entity_type=entity_type,
        entity_id=entity_id,
        request_id=request_id,
        metadata_json=json.dumps(metadata or {}, sort_keys=True),
    )
    db.add(event)
    return event
