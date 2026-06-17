from types import SimpleNamespace

from tests.helpers import (
    auth_headers,
    login_user,
    register_and_login,
    register_user,
)


def test_login_attempt_limiter_expires_old_failures(monkeypatch):
    from app.auth import rate_limit
    from app.auth.rate_limit import LoginAttemptLimiter

    current_time = 0.0
    monkeypatch.setattr(rate_limit.time, "monotonic", lambda: current_time)

    request = SimpleNamespace(client=SimpleNamespace(host="testclient"))
    limiter = LoginAttemptLimiter(max_attempts=1, window_seconds=10)

    limiter.record_failure(request, "expired-login@example.com")
    assert limiter.is_limited(request, "expired-login@example.com")

    current_time = 11.0

    assert not limiter.is_limited(request, "expired-login@example.com")
    assert "testclient:expired-login@example.com" not in limiter.failed_attempts


def test_login_failures_are_throttled_per_client_and_email(client, monkeypatch):
    from app.auth import routes as auth_routes
    from app.auth.rate_limit import LoginAttemptLimiter

    monkeypatch.setattr(
        auth_routes,
        "login_attempt_limiter",
        LoginAttemptLimiter(max_attempts=2, window_seconds=60),
    )

    register_response = register_user(client, "throttled-login@example.com")
    first_failure = login_user(client, "throttled-login@example.com", "WrongPassword1!")
    second_failure = login_user(client, "throttled-login@example.com", "WrongPassword1!")
    throttled_response = login_user(client, "throttled-login@example.com", "WrongPassword1!")

    assert register_response.status_code == 201
    assert first_failure.status_code == 401
    assert second_failure.status_code == 401
    assert throttled_response.status_code == 429
    assert throttled_response.json() == {
        "detail": "Too many failed login attempts",
        "limit": 2,
        "window_seconds": 60,
    }


def test_successful_login_clears_prior_failed_login_attempts(client, monkeypatch):
    from app.auth import routes as auth_routes
    from app.auth.rate_limit import LoginAttemptLimiter

    monkeypatch.setattr(
        auth_routes,
        "login_attempt_limiter",
        LoginAttemptLimiter(max_attempts=2, window_seconds=60),
    )

    register_response = register_user(client, "cleared-login@example.com")
    first_failure = login_user(client, "cleared-login@example.com", "WrongPassword1!")
    successful_login = login_user(client, "cleared-login@example.com")
    second_failure = login_user(client, "cleared-login@example.com", "WrongPassword1!")

    assert register_response.status_code == 201
    assert first_failure.status_code == 401
    assert successful_login.status_code == 200
    assert second_failure.status_code == 401


def test_register_login_and_get_current_user(client):
    token = register_and_login(client, "auth-flow@example.com")

    response = client.get("/users/me", headers=auth_headers(token))

    assert response.status_code == 200
    assert response.json() == {
        "user": {
            "id": 1,
            "email": "auth-flow@example.com",
        },
    }
    assert "password_hash" not in response.text


def test_duplicate_registration_returns_conflict(client):
    first_response = register_user(client, "duplicate@example.com")
    second_response = register_user(client, "duplicate@example.com")

    assert first_response.status_code == 201
    assert second_response.status_code == 409
    assert second_response.json()["detail"] == "Email is already registered"


def test_email_is_normalized_before_storage_and_login(client):
    register_response = register_user(client, "Mixed.Case@Example.COM")
    login_response = login_user(client, "mixed.case@example.com")

    assert register_response.status_code == 201
    assert register_response.json()["user"]["email"] == "mixed.case@example.com"
    assert login_response.status_code == 200


def test_invalid_login_does_not_reveal_which_field_failed(client):
    register_response = register_user(client, "login-failure@example.com")
    login_response = login_user(client, "login-failure@example.com", "WrongPassword1!")

    assert register_response.status_code == 201
    assert login_response.status_code == 401
    assert login_response.json()["detail"] == "Invalid email or password"


def test_registration_rejects_weak_passwords(client):
    response = register_user(client, "weak-password@example.com", "password")

    assert response.status_code == 422
