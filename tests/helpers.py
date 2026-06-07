from fastapi.testclient import TestClient

DEFAULT_PASSWORD = "Password1!"


def auth_headers(token: str, **extra_headers: str) -> dict[str, str]:
    return {"Authorization": f"Bearer {token}", **extra_headers}


def register_user(client: TestClient, email: str, password: str = DEFAULT_PASSWORD):
    return client.post(
        "/auth/register",
        json={"email": email, "password": password},
    )


def login_user(client: TestClient, email: str, password: str = DEFAULT_PASSWORD):
    return client.post(
        "/auth/login",
        json={"email": email, "password": password},
    )


def register_and_login(client: TestClient, email: str, password: str = DEFAULT_PASSWORD) -> str:
    register_response = register_user(client, email, password)
    assert register_response.status_code == 201

    login_response = login_user(client, email, password)
    assert login_response.status_code == 200
    return login_response.json()["access_token"]


def create_merchant(
    client: TestClient,
    token: str,
    *,
    name: str = "Summit Coffee",
    category: str = "food",
    trust_status: str = "trusted",
) -> dict:
    response = client.post(
        "/merchants",
        headers=auth_headers(token),
        json={"name": name, "category": category, "trust_status": trust_status},
    )
    assert response.status_code == 201
    return response.json()
