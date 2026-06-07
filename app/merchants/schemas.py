from datetime import datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, field_validator


def reject_control_characters(value: str) -> str:
    cleaned = value.strip()
    if any(ord(char) < 32 or ord(char) == 127 for char in cleaned):
        raise ValueError("Value cannot contain control characters")
    if not cleaned:
        raise ValueError("Value cannot be blank")
    return cleaned


class MerchantCreateRequest(BaseModel):
    model_config = ConfigDict(extra="forbid", str_strip_whitespace=True)

    name: str = Field(min_length=1, max_length=255)
    category: str = Field(min_length=1, max_length=100, pattern=r"^[A-Za-z0-9 _-]+$")
    trust_status: Literal["trusted", "untrusted"] = "trusted"

    @field_validator("name")
    @classmethod
    def clean_name(cls, value: str) -> str:
        return reject_control_characters(value)

    @field_validator("category")
    @classmethod
    def clean_category(cls, value: str) -> str:
        return reject_control_characters(value).lower()

    @field_validator("trust_status", mode="before")
    @classmethod
    def clean_trust_status(cls, value: str) -> str:
        if not isinstance(value, str):
            return value
        return reject_control_characters(value).lower()


class MerchantResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    owner_user_id: int
    name: str
    category: str
    trust_status: str
    created_at: datetime
    updated_at: datetime


class MerchantListResponse(BaseModel):
    merchants: list[MerchantResponse]
