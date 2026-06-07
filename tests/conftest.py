import os
import shutil
import tempfile
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

TEST_DATABASE_DIR = Path(tempfile.mkdtemp(prefix="clearance-tests-"))
TEST_DATABASE_PATH = TEST_DATABASE_DIR / "clearance.sqlite3"

os.environ["DATABASE_URL"] = f"sqlite:///{TEST_DATABASE_PATH}"
os.environ["SECRET_KEY"] = "test-secret-key-with-more-than-thirty-two-characters"
os.environ["AUTO_CREATE_TABLES"] = "true"
os.environ["ENABLE_DOCS"] = "false"
os.environ["RATE_LIMIT_MAX_REQUESTS"] = "1000"
os.environ["RATE_LIMIT_WINDOW_SECONDS"] = "60"
os.environ["MAX_REQUEST_BODY_BYTES"] = "2048"

from app.db.session import Base, engine  # noqa: E402
from app.main import app  # noqa: E402


@pytest.fixture()
def reset_database():
    Base.metadata.drop_all(bind=engine)
    Base.metadata.create_all(bind=engine)
    yield


@pytest.fixture()
def client(reset_database):
    with TestClient(app) as test_client:
        yield test_client


def pytest_sessionfinish(session, exitstatus):
    engine.dispose()
    shutil.rmtree(TEST_DATABASE_DIR, ignore_errors=True)
