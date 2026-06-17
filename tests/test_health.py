from app import main as app_main


def test_database_health_check_reports_ready(client):
    response = client.get("/health/db")

    assert response.status_code == 200
    assert response.json() == {"status": "ok", "database": "ready"}


def test_database_health_check_is_not_rate_limited(client):
    response = client.get("/health/db")

    assert response.status_code == 200
    assert "x-ratelimit-limit" not in response.headers
    assert "x-ratelimit-remaining" not in response.headers


def test_database_health_check_reports_unavailable(client, monkeypatch):
    monkeypatch.setattr(app_main, "check_database_health", lambda: False, raising=False)

    response = client.get("/health/db")

    assert response.status_code == 503
    assert response.json() == {"status": "error", "database": "unavailable"}
