from datetime import datetime

from pydantic import BaseModel, ConfigDict


class AuditEventResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    user_id: int | None
    action: str
    entity_type: str
    entity_id: int | None
    request_id: str | None
    metadata_json: str
    created_at: datetime


class AuditEventListResponse(BaseModel):
    audit_events: list[AuditEventResponse]
