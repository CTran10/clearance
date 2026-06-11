import os
import uuid
from decimal import Decimal

import pytest
from alembic import command
from alembic.config import Config
from sqlalchemy import create_engine, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import sessionmaker

from app.db.models import Merchant, Transaction, User

POSTGRES_DATABASE_URL = os.getenv("POSTGRES_INTEGRATION_DATABASE_URL")

pytestmark = [
    pytest.mark.integration,
    pytest.mark.skipif(
        not POSTGRES_DATABASE_URL,
        reason="POSTGRES_INTEGRATION_DATABASE_URL is not set",
    ),
]


@pytest.fixture()
def postgres_engine():
    schema_name = f"clearance_test_{uuid.uuid4().hex}"
    admin_engine = create_engine(POSTGRES_DATABASE_URL, pool_pre_ping=True)

    with admin_engine.begin() as connection:
        connection.execute(text(f'CREATE SCHEMA "{schema_name}"'))

    # used to do Base.metadata.create_all() here and it felt great until i realized:
    # that builds the schema straight from my models, totally ignoring my actual migration files.
    # so the tests were green while a missing migration would've nuked prod. now we run the REAL migrations
    # against a throwaway schema — if alembic is broken, the test screams here instead of at 2am
    run_migrations(schema_name)
    test_engine = create_engine(
        POSTGRES_DATABASE_URL,
        connect_args={"options": f"-csearch_path={schema_name}"},
        pool_pre_ping=True,
    )

    try:
        yield test_engine
    finally:
        test_engine.dispose()
        with admin_engine.begin() as connection:
            connection.execute(text(f'DROP SCHEMA IF EXISTS "{schema_name}" CASCADE'))
        admin_engine.dispose()


def run_migrations(schema_name: str) -> None:
    config = Config("alembic.ini")
    config.set_main_option("sqlalchemy.url", POSTGRES_DATABASE_URL)
    config.attributes["connect_args"] = {"options": f"-csearch_path={schema_name}"}
    command.upgrade(config, "head")


def test_alembic_migrations_create_current_postgres_schema(postgres_engine):
    session_factory = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=postgres_engine,
    )

    db = session_factory()
    try:
        table_names = {
            row[0]
            for row in db.execute(
                text(
                    "select tablename "
                    "from pg_tables "
                    "where schemaname = current_schema()"
                )
            )
        }

        assert {"users", "merchants", "transactions", "audit_events"}.issubset(table_names)
    finally:
        db.close()


def test_postgres_enforces_transaction_idempotency_constraint(postgres_engine):
    session_factory = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=postgres_engine,
    )

    db = session_factory()
    try:
        user = User(
            email="postgres-constraint@example.com",
            password_hash="not-a-real-password-hash",
        )
        db.add(user)
        db.flush()

        merchant = Merchant(
            owner_user_id=user.id,
            name="Postgres Merchant",
            category="retail",
            trust_status="trusted",
        )
        db.add(merchant)
        db.flush()

        first_transaction = build_transaction(user, merchant, "duplicate-key")
        second_transaction = build_transaction(user, merchant, "duplicate-key")
        db.add_all([first_transaction, second_transaction])

        with pytest.raises(IntegrityError):
            db.commit()
    finally:
        db.rollback()
        db.close()


def test_postgres_stores_money_with_two_decimal_places(postgres_engine):
    session_factory = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=postgres_engine,
    )

    db = session_factory()
    try:
        user = User(
            email="postgres-numeric@example.com",
            password_hash="not-a-real-password-hash",
        )
        db.add(user)
        db.flush()

        merchant = Merchant(
            owner_user_id=user.id,
            name="Numeric Merchant",
            category="retail",
            trust_status="trusted",
        )
        db.add(merchant)
        db.flush()

        transaction = build_transaction(user, merchant, "numeric-key")
        db.add(transaction)
        db.commit()
        db.refresh(transaction)

        assert transaction.amount == Decimal("12.35")
    finally:
        db.rollback()
        db.close()


def build_transaction(user: User, merchant: Merchant, idempotency_key: str) -> Transaction:
    return Transaction(
        user_id=user.id,
        merchant_id=merchant.id,
        amount=Decimal("12.345"),
        currency="USD",
        status="approved",
        risk_score=20,
        decision_reason="Postgres integration test",
        idempotency_key=idempotency_key,
    )
