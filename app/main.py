import logging

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.audit.routes import router as audit_router
from app.auth.routes import router as auth_router
from app.core.config import settings
from app.merchants.routes import router as merchants_router
from app.middleware.body_size import BodySizeLimitMiddleware
from app.middleware.rate_limit import RateLimitMiddleware
from app.middleware.request_logging import RequestLoggingMiddleware
from app.middleware.security_headers import SecurityHeadersMiddleware
from app.transactions.routes import router as transactions_router
from app.users.routes import router as users_router

from app.db import models
from app.db.session import Base, engine


def create_app() -> FastAPI:
    app = FastAPI(
        docs_url="/docs" if settings.enable_docs else None,
        redoc_url="/redoc" if settings.enable_docs else None,
        openapi_url="/openapi.json" if settings.enable_docs else None,
    )

    configure_logging()
    configure_database()
    configure_middleware(app)
    include_routers(app)
    register_health_route(app)

    return app


def configure_logging() -> None:
    logging.basicConfig(level=logging.INFO)


def configure_database() -> None:
    if settings.auto_create_tables:
        Base.metadata.create_all(bind=engine)


def configure_middleware(app: FastAPI) -> None:
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.cors_origins,
        allow_credentials=True,
        allow_methods=["GET", "POST", "OPTIONS"],
        allow_headers=["Authorization", "Content-Type", "Idempotency-Key", "X-Request-ID"],
    )
    app.add_middleware(
        BodySizeLimitMiddleware,
        max_body_bytes=settings.max_request_body_bytes,
    )
    app.add_middleware(
        RateLimitMiddleware,
        max_requests=settings.rate_limit_max_requests,
        window_seconds=settings.rate_limit_window_seconds,
        trust_proxy_headers=settings.trust_proxy_headers,
        trusted_proxy_cidrs=settings.trusted_proxy_cidrs,
        excluded_paths={"/health"},
    )
    app.add_middleware(RequestLoggingMiddleware)
    app.add_middleware(SecurityHeadersMiddleware)


def include_routers(app: FastAPI) -> None:
    app.include_router(auth_router)
    app.include_router(users_router)
    app.include_router(merchants_router)
    app.include_router(transactions_router)
    app.include_router(audit_router)


def register_health_route(app: FastAPI) -> None:
    @app.get("/health")
    def health():
        return {"status": "ok"}


app = create_app()
