import re

import anyio
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse
from starlette.routing import Route
from fastapi.testclient import TestClient

from app.middleware.body_size import BodySizeLimitMiddleware
from app.middleware.rate_limit import RateLimitMiddleware
from app.middleware.security_headers import SecurityHeadersMiddleware
from tests.helpers import auth_headers, create_merchant, register_and_login, register_user

REQUEST_ID_PATTERN = re.compile(r"^[A-Za-z0-9._:-]{1,64}$")
SECURITY_HEADERS = {
    "X-Content-Type-Options": "nosniff",
    "X-Frame-Options": "DENY",
    "Cache-Control": "no-store",
}


def test_invalid_request_id_header_is_not_echoed(client):
    response = client.get("/health", headers={"X-Request-ID": "bad\nrequest"})
    request_id = response.headers["X-Request-ID"]

    assert response.status_code == 200
    assert request_id != "bad\nrequest"
    assert REQUEST_ID_PATTERN.fullmatch(request_id)


def test_request_body_size_limit_runs_before_route_validation(client):
    response = client.post(
        "/auth/register",
        json={
            "email": "large-body@example.com",
            "password": "Password1!" + ("x" * 3000),
        },
    )

    assert response.status_code == 413
    assert response.json()["detail"] == "Request body is too large"
    assert_security_headers(response.headers)


def test_request_body_size_limit_counts_streamed_body_without_content_length():
    async def run_request():
        async def read_body(request: Request):
            body = await request.body()
            return JSONResponse({"size": len(body)})

        app = Starlette(routes=[Route("/", read_body, methods=["POST"])])
        app.add_middleware(BodySizeLimitMiddleware, max_body_bytes=4)

        messages = [
            {"type": "http.request", "body": b"123", "more_body": True},
            {"type": "http.request", "body": b"45", "more_body": False},
        ]
        sent_messages = []

        async def receive():
            return messages.pop(0)

        async def send(message):
            sent_messages.append(message)

        await app(build_post_scope(headers=[]), receive, send)
        return sent_messages

    sent_messages = anyio.run(run_request)

    response_start = sent_messages[0]
    assert response_start["type"] == "http.response.start"
    assert response_start["status"] == 413


def test_invalid_authorization_header_is_rejected(client):
    response = client.get("/users/me", headers={"Authorization": "Token abc"})

    assert response.status_code == 401
    assert response.json()["detail"] == "Invalid Authorization header"


def test_idempotency_key_is_required_for_transaction_creation(client):
    token = register_and_login(client, "missing-idempotency@example.com")
    merchant = create_merchant(client, token)

    response = client.post(
        "/transactions",
        headers=auth_headers(token),
        json={"merchant_id": merchant["id"], "amount": "10.00", "currency": "USD"},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Idempotency-Key header is required"


def test_merchant_input_rejects_control_characters(client):
    token = register_and_login(client, "merchant-validation@example.com")

    response = client.post(
        "/merchants",
        headers=auth_headers(token),
        json={"name": "Bad\nName", "category": "retail"},
    )

    assert response.status_code == 422


def test_transaction_input_rejects_invalid_currency(client):
    token = register_and_login(client, "currency-validation@example.com")
    merchant = create_merchant(client, token)

    response = client.post(
        "/transactions",
        headers=auth_headers(token, **{"Idempotency-Key": "bad-currency"}),
        json={"merchant_id": merchant["id"], "amount": "10.00", "currency": "US1"},
    )

    assert response.status_code == 422


def test_request_models_reject_unknown_fields(client):
    register_response = client.post(
        "/auth/register",
        json={
            "email": "extra-field@example.com",
            "password": "Password1!",
            "role": "admin",
        },
    )

    token = register_and_login(client, "extra-field-owner@example.com")
    merchant_response = client.post(
        "/merchants",
        headers=auth_headers(token),
        json={
            "name": "Extra Field Shop",
            "category": "retail",
            "owner_user_id": 999,
        },
    )

    assert register_response.status_code == 422
    assert merchant_response.status_code == 422


def test_long_password_is_rejected_before_hashing(client):
    response = register_user(
        client,
        "long-password@example.com",
        "Password1!" + ("x" * 128),
    )

    assert response.status_code == 422


def test_rate_limit_uses_direct_client_when_proxy_headers_are_not_trusted():
    async def ok(_request):
        return JSONResponse({"ok": True})

    app = Starlette(routes=[Route("/", ok)])
    app.add_middleware(
        RateLimitMiddleware,
        max_requests=1,
        window_seconds=60,
        trust_proxy_headers=False,
    )

    with TestClient(app) as test_client:
        first_response = test_client.get("/", headers={"X-Forwarded-For": "1.1.1.1"})
        second_response = test_client.get("/", headers={"X-Forwarded-For": "2.2.2.2"})

    assert first_response.status_code == 200
    assert second_response.status_code == 429


def test_rate_limit_only_trusts_forwarded_for_from_configured_proxy():
    app = create_rate_limited_app(trust_proxy_headers=True, trusted_proxy_cidrs=["10.0.0.0/8"])

    with TestClient(app, client=("10.1.2.3", 12345)) as test_client:
        first_response = test_client.get("/", headers={"X-Forwarded-For": "1.1.1.1"})
        second_response = test_client.get("/", headers={"X-Forwarded-For": "2.2.2.2"})

    assert first_response.status_code == 200
    assert second_response.status_code == 200


def test_rate_limit_generated_response_includes_security_headers():
    app = create_rate_limited_app()

    with TestClient(app) as test_client:
        first_response = test_client.get("/")
        second_response = test_client.get("/")

    assert first_response.status_code == 200
    assert second_response.status_code == 429
    assert_security_headers(second_response.headers)


def create_rate_limited_app(
    *,
    trust_proxy_headers: bool = False,
    trusted_proxy_cidrs: list[str] | None = None,
) -> Starlette:
    async def ok(_request):
        return JSONResponse({"ok": True})

    app = Starlette(routes=[Route("/", ok)])
    app.add_middleware(
        RateLimitMiddleware,
        max_requests=1,
        window_seconds=60,
        trust_proxy_headers=trust_proxy_headers,
        trusted_proxy_cidrs=trusted_proxy_cidrs,
    )
    app.add_middleware(SecurityHeadersMiddleware)
    return app


def assert_security_headers(headers) -> None:
    for header, expected_value in SECURITY_HEADERS.items():
        assert headers[header] == expected_value


def build_post_scope(*, headers: list[tuple[bytes, bytes]]):
    return {
        "type": "http",
        "asgi": {"version": "3.0"},
        "http_version": "1.1",
        "method": "POST",
        "scheme": "http",
        "path": "/",
        "raw_path": b"/",
        "query_string": b"",
        "headers": headers,
        "client": ("testclient", 50000),
        "server": ("testserver", 80),
        "root_path": "",
    }
