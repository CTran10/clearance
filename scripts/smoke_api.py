from __future__ import annotations

import os
import sys
import time
import uuid
from typing import Any

import httpx

DEFAULT_BASE_URL = "http://127.0.0.1:8000"
PASSWORD = "Password1!"


class SmokeFailure(AssertionError):
    pass


def main() -> int:
    base_url = normalize_base_url(os.getenv("CLEARANCE_API_BASE_URL", DEFAULT_BASE_URL))
    run_id = uuid.uuid4().hex[:10]
    email = f"smoke-{run_id}@example.com"

    with httpx.Client(base_url=base_url, timeout=8.0) as client:
        wait_for_api(client)
        token = register_and_login(client, email)
        headers = {"Authorization": f"Bearer {token}"}

        trusted_merchant = create_merchant(
            client,
            headers,
            name=f"Smoke Trusted {run_id}",
            category="retail",
            trust_status="trusted",
        )
        untrusted_merchant = create_merchant(
            client,
            headers,
            name=f"Smoke Untrusted {run_id}",
            category="wire_transfer",
            trust_status="untrusted",
        )

        approved = create_transaction(
            client,
            headers,
            idempotency_key=f"smoke-approved-{run_id}",
            merchant_id=trusted_merchant["id"],
            amount="42.00",
            expected_status_code=201,
        )
        assert_field(approved, "status", "approved")

        # the money shot of the whole project: same idempotency key + same body = 200 (not 201!) and the
        # SAME transaction id comes back. note the status code flip — 201 "created" the first time, 200 "here's
        # the one you already made" on retry. if this ever returns a new id, we're double-charging people
        retry = create_transaction(
            client,
            headers,
            idempotency_key=f"smoke-approved-{run_id}",
            merchant_id=trusted_merchant["id"],
            amount="42.00",
            expected_status_code=200,
        )
        assert_field(retry, "id", approved["id"])

        # ...but same key + DIFFERENT body (43 vs 42) must blow up with 409. "you already used this key for
        # something else, i'm not guessing which one you meant". this is the line that proves the hash check works
        conflict = client.post(
            "/transactions",
            headers={**headers, "Idempotency-Key": f"smoke-approved-{run_id}"},
            json={
                "merchant_id": trusted_merchant["id"],
                "amount": "43.00",
                "currency": "USD",
            },
        )
        assert_status(conflict, 409)

        review = create_transaction(
            client,
            headers,
            idempotency_key=f"smoke-review-{run_id}",
            merchant_id=untrusted_merchant["id"],
            amount="25.00",
            expected_status_code=201,
        )
        assert_field(review, "status", "review")

        declined = create_transaction(
            client,
            headers,
            idempotency_key=f"smoke-decline-{run_id}",
            merchant_id=trusted_merchant["id"],
            amount="10000.00",
            expected_status_code=201,
        )
        assert_field(declined, "status", "declined")

        audit_events = get_json(client.get("/audit-events?limit=100", headers=headers))[
            "audit_events"
        ]
        require_audit_actions(
            audit_events,
            {
                "REGISTERED_USER",
                "LOGGED_IN",
                "CREATED_MERCHANT",
                "TRANSACTION_APPROVED",
                "TRANSACTION_REVIEW",
                "TRANSACTION_DECLINED",
            },
        )

    print(
        "Smoke PASS: register/login, merchant creation, idempotent retry, "
        "conflict handling, risk decisions, and audit events all worked."
    )
    return 0


def normalize_base_url(value: str) -> str:
    return value.strip().rstrip("/") or DEFAULT_BASE_URL


def wait_for_api(client: httpx.Client, *, attempts: int = 30, delay_seconds: float = 0.5) -> None:
    last_error: Exception | None = None
    for _ in range(attempts):
        try:
            health = client.get("/health")
            database_health = client.get("/health/db")
            if health.status_code == 200 and database_health.status_code == 200:
                return
        except httpx.HTTPError as exc:
            last_error = exc
        time.sleep(delay_seconds)

    message = "API did not become healthy before the smoke timeout"
    if last_error:
        message = f"{message}: {last_error}"
    raise SmokeFailure(message)


def register_and_login(client: httpx.Client, email: str) -> str:
    register_response = client.post(
        "/auth/register",
        json={"email": email, "password": PASSWORD},
    )
    assert_status(register_response, 201)

    login_response = client.post(
        "/auth/login",
        json={"email": email, "password": PASSWORD},
    )
    login_payload = get_json(login_response)
    token = login_payload.get("access_token")
    if not isinstance(token, str) or not token:
        raise SmokeFailure("Login response did not include an access token")
    return token


def create_merchant(
    client: httpx.Client,
    headers: dict[str, str],
    *,
    name: str,
    category: str,
    trust_status: str,
) -> dict[str, Any]:
    return get_json(
        client.post(
            "/merchants",
            headers=headers,
            json={
                "name": name,
                "category": category,
                "trust_status": trust_status,
            },
        ),
        expected_status_code=201,
    )


def create_transaction(
    client: httpx.Client,
    headers: dict[str, str],
    *,
    idempotency_key: str,
    merchant_id: int,
    amount: str,
    expected_status_code: int,
) -> dict[str, Any]:
    return get_json(
        client.post(
            "/transactions",
            headers={**headers, "Idempotency-Key": idempotency_key},
            json={
                "merchant_id": merchant_id,
                "amount": amount,
                "currency": "USD",
            },
        ),
        expected_status_code=expected_status_code,
    )


def get_json(response: httpx.Response, *, expected_status_code: int = 200) -> dict[str, Any]:
    assert_status(response, expected_status_code)
    payload = response.json()
    if not isinstance(payload, dict):
        raise SmokeFailure(f"Expected JSON object, received {type(payload).__name__}")
    return payload


def assert_status(response: httpx.Response, expected_status_code: int) -> None:
    if response.status_code != expected_status_code:
        raise SmokeFailure(
            f"{response.request.method} {response.request.url} returned "
            f"{response.status_code}, expected {expected_status_code}: {response.text}"
        )


def assert_field(payload: dict[str, Any], field: str, expected: Any) -> None:
    actual = payload.get(field)
    if actual != expected:
        raise SmokeFailure(f"Expected {field}={expected!r}, received {actual!r}")


def require_audit_actions(audit_events: list[dict[str, Any]], expected_actions: set[str]) -> None:
    actions = {event.get("action") for event in audit_events}
    missing_actions = expected_actions - actions
    if missing_actions:
        raise SmokeFailure(f"Audit events missing actions: {sorted(missing_actions)}")


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except SmokeFailure as exc:
        print(f"Smoke FAIL: {exc}", file=sys.stderr)
        raise SystemExit(1)
