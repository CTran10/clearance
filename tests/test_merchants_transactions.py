from decimal import Decimal

from app.db.models import Transaction
from app.db.session import SessionLocal
from app.transactions.schemas import TransactionCreateRequest
from app.transactions.service import build_idempotency_request_hash
from tests.helpers import auth_headers, create_merchant, register_and_login


def test_merchants_are_scoped_to_the_authenticated_user(client):
    first_token = register_and_login(client, "merchant-owner@example.com")
    second_token = register_and_login(client, "merchant-viewer@example.com")

    create_merchant(client, first_token, name="Owner Shop", category="retail")

    first_response = client.get("/merchants", headers=auth_headers(first_token))
    second_response = client.get("/merchants", headers=auth_headers(second_token))

    assert first_response.status_code == 200
    assert second_response.status_code == 200
    assert len(first_response.json()["merchants"]) == 1
    assert second_response.json()["merchants"] == []
    assert first_response.json()["merchants"][0]["trust_status"] == "trusted"


def test_transaction_creation_is_idempotent_for_same_payload(client):
    token = register_and_login(client, "idempotent@example.com")
    merchant = create_merchant(client, token)
    payload = {"merchant_id": merchant["id"], "amount": "125.50", "currency": "usd"}
    headers = auth_headers(token, **{"Idempotency-Key": "txn-key-001"})

    first_response = client.post("/transactions", headers=headers, json=payload)
    retry_response = client.post("/transactions", headers=headers, json=payload)

    assert first_response.status_code == 201
    assert retry_response.status_code == 200
    assert retry_response.json()["id"] == first_response.json()["id"]
    assert retry_response.json()["currency"] == "USD"
    assert "idempotency_key" not in first_response.json()


def test_transaction_creation_stores_canonical_idempotency_request_hash(client):
    token = register_and_login(client, "idempotency-hash@example.com")
    merchant = create_merchant(client, token)
    payload = {"merchant_id": merchant["id"], "amount": "42.00", "currency": "usd"}

    response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "txn-key-hash"}),
        json=payload,
    )

    assert response.status_code == 201
    assert "idempotency_request_hash" not in response.json()

    db = SessionLocal()
    try:
        transaction = (
            db.query(Transaction)
            .filter(Transaction.id == response.json()["id"])
            .one()
        )
        expected_hash = build_idempotency_request_hash(
            TransactionCreateRequest(
                merchant_id=merchant["id"],
                amount=Decimal("42.00"),
                currency="USD",
            )
        )
        assert transaction.idempotency_request_hash == expected_hash
        assert len(transaction.idempotency_request_hash) == 64
    finally:
        db.close()


def test_idempotency_request_hash_uses_canonical_payload_values():
    first_hash = build_idempotency_request_hash(
        TransactionCreateRequest(merchant_id=1, amount=Decimal("42.0"), currency="usd")
    )
    matching_hash = build_idempotency_request_hash(
        TransactionCreateRequest(merchant_id=1, amount=Decimal("42.00"), currency="USD")
    )
    different_hash = build_idempotency_request_hash(
        TransactionCreateRequest(merchant_id=1, amount=Decimal("42.01"), currency="USD")
    )

    assert first_hash == matching_hash
    assert first_hash != different_hash


def test_reusing_idempotency_key_with_different_payload_returns_conflict(client):
    token = register_and_login(client, "idempotency-conflict@example.com")
    merchant = create_merchant(client, token)
    headers = auth_headers(token, **{"Idempotency-Key": "txn-key-002"})

    first_response = client.post(
        "/transactions",
        headers=headers,
        json={"merchant_id": merchant["id"], "amount": "125.50", "currency": "USD"},
    )
    conflict_response = client.post(
        "/transactions",
        headers=headers,
        json={"merchant_id": merchant["id"], "amount": "126.50", "currency": "USD"},
    )

    assert first_response.status_code == 201
    assert conflict_response.status_code == 409
    assert conflict_response.json()["detail"] == (
        "Idempotency-Key was already used with a different payload"
    )


def test_transaction_cannot_use_another_users_merchant(client):
    owner_token = register_and_login(client, "merchant-owner-tx@example.com")
    other_token = register_and_login(client, "merchant-attacker@example.com")
    merchant = create_merchant(client, owner_token)

    response = client.post(
        "/transactions",
        headers=auth_headers(other_token, **{"Idempotency-Key": "cross-user-merchant"}),
        json={"merchant_id": merchant["id"], "amount": "10.00", "currency": "USD"},
    )

    assert response.status_code == 404
    assert response.json()["detail"] == "Merchant not found"


def test_risk_decisions_follow_current_threshold_rules(client):
    token = register_and_login(client, "risk-rules@example.com")
    merchant = create_merchant(client, token)

    review_response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "risk-review"}),
        json={"merchant_id": merchant["id"], "amount": "5000.00", "currency": "USD"},
    )
    declined_response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "risk-decline"}),
        json={"merchant_id": merchant["id"], "amount": "10000.00", "currency": "USD"},
    )

    assert review_response.status_code == 201
    assert review_response.json()["status"] == "review"
    assert declined_response.status_code == 201
    assert declined_response.json()["status"] == "declined"


def test_untrusted_merchant_transactions_go_to_review(client):
    token = register_and_login(client, "untrusted-merchant@example.com")
    merchant = create_merchant(
        client,
        token,
        name="New Wire Merchant",
        category="retail",
        trust_status="untrusted",
    )

    response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "untrusted-merchant"}),
        json={"merchant_id": merchant["id"], "amount": "25.00", "currency": "USD"},
    )

    assert response.status_code == 201
    assert response.json()["status"] == "review"
    assert response.json()["risk_score"] == 80
    assert response.json()["decision_reason"] == "Merchant is marked untrusted"


def test_transaction_velocity_goes_to_review_after_recent_activity(client):
    token = register_and_login(client, "velocity-review@example.com")
    merchant = create_merchant(client, token)

    for transaction_number in range(5):
        response = client.post(
            "/transactions",
            headers=auth_headers(
                token,
                **{"Idempotency-Key": f"velocity-{transaction_number}"},
            ),
            json={"merchant_id": merchant["id"], "amount": "10.00", "currency": "USD"},
        )
        assert response.status_code == 201

    velocity_response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "velocity-review"}),
        json={"merchant_id": merchant["id"], "amount": "10.00", "currency": "USD"},
    )

    assert velocity_response.status_code == 201
    assert velocity_response.json()["status"] == "review"
    assert velocity_response.json()["risk_score"] == 65
    assert velocity_response.json()["decision_reason"] == (
        "Too many recent transactions for this user"
    )


def test_transactions_are_scoped_to_the_authenticated_user(client):
    first_token = register_and_login(client, "transaction-owner@example.com")
    second_token = register_and_login(client, "transaction-viewer@example.com")
    merchant = create_merchant(client, first_token)

    create_response = client.post(
        "/transactions",
        headers=auth_headers(first_token, **{"Idempotency-Key": "owned-transaction"}),
        json={"merchant_id": merchant["id"], "amount": "50.00", "currency": "USD"},
    )
    transaction_id = create_response.json()["id"]

    list_response = client.get("/transactions", headers=auth_headers(second_token))
    get_response = client.get(
        f"/transactions/{transaction_id}",
        headers=auth_headers(second_token),
    )

    assert create_response.status_code == 201
    assert list_response.status_code == 200
    assert list_response.json()["transactions"] == []
    assert get_response.status_code == 404


def test_audit_events_record_user_domain_actions(client):
    token = register_and_login(client, "audit-events@example.com")
    merchant = create_merchant(client, token)

    transaction_response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "audit-transaction"}),
        json={"merchant_id": merchant["id"], "amount": "50.00", "currency": "USD"},
    )
    audit_response = client.get("/audit-events", headers=auth_headers(token))

    assert transaction_response.status_code == 201
    assert audit_response.status_code == 200

    audit_events = audit_response.json()["audit_events"]
    actions = {event["action"] for event in audit_events}
    assert {
        "REGISTERED_USER",
        "LOGGED_IN",
        "CREATED_MERCHANT",
        "TRANSACTION_APPROVED",
    }.issubset(actions)
    assert all(event["user_id"] == 1 for event in audit_events)
