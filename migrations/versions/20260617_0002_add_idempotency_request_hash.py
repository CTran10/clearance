"""add idempotency request hash to transactions

Revision ID: 20260617_0002
Revises: 20260606_0001
Create Date: 2026-06-17 00:00:00.000000
"""
from collections.abc import Sequence

from alembic import op
import sqlalchemy as sa

revision: str = "20260617_0002"
down_revision: str | None = "20260606_0001"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "transactions",
        sa.Column("idempotency_request_hash", sa.String(length=64), nullable=True),
    )


def downgrade() -> None:
    op.drop_column("transactions", "idempotency_request_hash")
