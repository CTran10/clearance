from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field


class MerchantCreateRequest(BaseModel):
    name: str = Field(min_length=1, max_length=255)
    category: str = Field(min_length=1, max_length=100)


class MerchantResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    owner_user_id: int
    name: str
    category: str
    created_at: datetime
    updated_at: datetime


class MerchantListResponse(BaseModel):
    merchants: list[MerchantResponse]
