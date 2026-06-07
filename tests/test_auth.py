from tests.helpers import (
    auth_headers,
    login_user,
    register_and_login,
    register_user,
)


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
