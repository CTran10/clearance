import os
import uuid
from concurrent.futures import ThreadPoolExecutor
from decimal import Decimal
from threading import Barrier

import pytest
from alembic import command
from alembic.config import Config
from sqlalchemy import create_engine, text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import sessionmaker

from app.db.models import Merchant, Transaction, User
from app.transactions import service as transaction_service
from app.transactions.schemas import TransactionCreateRequest

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


def test_postgres_concurrent_transaction_creation_is_idempotent(
    postgres_engine,
    monkeypatch,
):
    session_factory = sessionmaker(
        autocommit=False,
        autoflush=False,
        bind=postgres_engine,
    )

    db = session_factory()
    try:
        user = User(
            email="postgres-concurrency@example.com",
            password_hash="not-a-real-password-hash",
        )
        db.add(user)
        db.flush()

        merchant = Merchant(
            owner_user_id=user.id,
            name="Concurrent Merchant",
            category="retail",
            trust_status="trusted",
        )
        db.add(merchant)
        db.commit()

        user_id = user.id
        merchant_id = merchant.id
    finally:
        db.close()

    payload = TransactionCreateRequest(
        merchant_id=merchant_id,
        amount=Decimal("42.00"),
        currency="USD",
    )
    # ok this is the coolest trick i learned this whole project. races are normally impossible to test
    # reliably bc the timing is random — the bug shows up 1 in 1000 runs and never when you're watching.
    # a Barrier(2) is like "both threads HAVE to hold hands and jump at the same time". so i force both
    # workers to finish the "nope, no existing txn here" lookup BEFORE either is allowed to insert.
    # that guarantees the exact collision i'm worried about, every single run. no more flaky maybe-bug
    empty_lookup_barrier = Barrier(2)
    original_find_transaction = transaction_service.find_transaction_by_idempotency_key

    def synchronized_empty_lookup(db, *, user_id: int, idempotency_key: str):
        transaction = original_find_transaction(
            db,
            user_id=user_id,
            idempotency_key=idempotency_key,
        )
        if transaction is None:
            empty_lookup_barrier.wait(timeout=15)
        return transaction

    monkeypatch.setattr(
        transaction_service,
        "find_transaction_by_idempotency_key",
        synchronized_empty_lookup,
    )

    def create_transaction() -> tuple[int, bool]:
        worker_db = session_factory()
        try:
            worker_user = worker_db.get(User, user_id)
            assert worker_user is not None

            transaction, created = transaction_service.create_transaction_with_idempotency(
                worker_db,
                user=worker_user,
                payload=payload,
                idempotency_key="concurrent-key",
                request_id="postgres-concurrency-test",
            )
            return transaction.id, created
        finally:
            worker_db.close()

    with ThreadPoolExecutor(max_workers=2) as executor:
        results = list(executor.map(lambda _: create_transaction(), range(2)))

    transaction_ids = {transaction_id for transaction_id, _ in results}
    created_flags = sorted(created for _, created in results)

    # the payoff: even though TWO threads both thought "i'm the first one!", they land on ONE transaction id.
    # exactly one of them actually created it (True), the other got told "lol no, here's the existing one" (False).
    # [False, True] sorted = one created, one deduped. zero double-charges. this is the whole point of the project tbh
    assert len(transaction_ids) == 1
    assert created_flags == [False, True]

    verification_db = session_factory()
    try:
        transactions = (
            verification_db.query(Transaction)
            .filter(
                Transaction.user_id == user_id,
                Transaction.idempotency_key == "concurrent-key",
            )
            .all()
        )
        assert len(transactions) == 1
    finally:
        verification_db.close()


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
