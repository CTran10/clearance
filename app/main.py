import logging
import os

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.audit.routes import router as audit_router
from app.auth.routes import router as auth_router
from app.merchants.routes import router as merchants_router
from app.middleware.rate_limit import RateLimitMiddleware
from app.middleware.request_logging import RequestLoggingMiddleware
from app.middleware.security_headers import SecurityHeadersMiddleware
from app.transactions.routes import router as transactions_router
from app.users.routes import router as users_router

app = FastAPI()
from app.db.session import Base, engine
from app.db import models

Base.metadata.create_all(bind=engine)

logging.basicConfig(level=logging.INFO)

cors_origins = [
    origin.strip()
    for origin in os.getenv("CORS_ORIGINS", "http://localhost:3000,http://127.0.0.1:3000").split(",")
    if origin.strip()
]

app.add_middleware(
    CORSMiddleware,
    allow_origins=cors_origins,
    allow_credentials=True,
    allow_methods=["GET", "POST", "OPTIONS"],
    allow_headers=["Authorization", "Content-Type", "Idempotency-Key", "X-Request-ID"],
)
app.add_middleware(SecurityHeadersMiddleware)
app.add_middleware(
    RateLimitMiddleware,
    max_requests=int(os.getenv("RATE_LIMIT_MAX_REQUESTS", "120")),
    window_seconds=int(os.getenv("RATE_LIMIT_WINDOW_SECONDS", "60")),
    excluded_paths={"/health"},
)
app.add_middleware(RequestLoggingMiddleware)

app.include_router(auth_router)
app.include_router(users_router)
app.include_router(merchants_router)
app.include_router(transactions_router)
app.include_router(audit_router)

# Basic health check for confirming the API process is running.
@app.get("/health")
def health():
    return {"status": "ok"}
