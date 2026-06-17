PYTHON ?= .venv/bin/python
API_BASE_URL ?= http://127.0.0.1:8000

.PHONY: install install-dev db-up run test coverage lint smoke frontend-test ci-local

install:
	$(PYTHON) -m pip install -r requirements.txt

install-dev:
	$(PYTHON) -m pip install -r requirements-dev.txt

db-up:
	docker compose up -d

run:
	$(PYTHON) -m uvicorn app.main:app --reload

test:
	$(PYTHON) -m pytest

coverage:
	$(PYTHON) -m coverage run -m pytest
	$(PYTHON) -m coverage report

lint:
	$(PYTHON) -m ruff check .
	$(PYTHON) -m ruff format --check .

smoke:
	CLEARANCE_API_BASE_URL=$(API_BASE_URL) $(PYTHON) scripts/smoke_api.py

frontend-test:
	cd frontend && npm test

ci-local: lint coverage frontend-test
