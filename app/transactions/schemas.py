from datetime import datetime
from decimal import Decimal

from pydantic import BaseModel, ConfigDict, Field, field_validator


class TransactionCreateRequest(BaseModel):
    merchant_id: int = Field(gt=0)
    amount: Decimal = Field(gt=0, max_digits=12, decimal_places=2)
    currency: str = Field(min_length=3, max_length=3)

    @field_validator("currency")
    @classmethod
    def normalize_currency(cls, value: str) -> str:
        return value.upper()


class TransactionResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: int
    user_id: int
    merchant_id: int
    amount: Decimal
    currency: str
    status: str
    risk_score: int
    decision_reason: str
    idempotency_key: str
    created_at: datetime
    updated_at: datetime


class TransactionListResponse(BaseModel):
    transactions: list[TransactionResponse]
