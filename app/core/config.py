import os
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation

from dotenv import load_dotenv

load_dotenv()


MIN_SECRET_KEY_LENGTH = 32


def _get_required_env(name: str) -> str:
    value = os.getenv(name)
    if not value:
        raise RuntimeError(f"{name} must be set")
    return value


def _get_positive_int_env(name: str, default: int) -> int:
    value = os.getenv(name)
    if value is None:
        return default
    try:
        parsed_value = int(value)
    except ValueError as exc:
        raise RuntimeError(f"{name} must be an integer") from exc

    if parsed_value <= 0:
        raise RuntimeError(f"{name} must be greater than 0")

    return parsed_value


def _get_positive_decimal_env(name: str, default: str) -> Decimal:
    value = os.getenv(name, default)
    try:
        parsed_value = Decimal(value)
    except InvalidOperation as exc:
        raise RuntimeError(f"{name} must be a decimal value") from exc

    if not parsed_value.is_finite():
        raise RuntimeError(f"{name} must be a decimal value")

    if parsed_value <= 0:
        raise RuntimeError(f"{name} must be greater than 0")

    return parsed_value


def _get_bool_env(name: str, default: bool = False) -> bool:
    value = os.getenv(name)
    if value is None:
        return default
    normalized_value = value.lower()
    if normalized_value in {"1", "true", "yes", "on"}:
        return True
    if normalized_value in {"0", "false", "no", "off"}:
        return False
    raise RuntimeError(f"{name} must be a boolean value")


def _get_csv_env(name: str) -> list[str]:
    value = os.getenv(name, "")
    return [item.strip() for item in value.split(",") if item.strip()]


def _get_secret_key() -> str:
    secret_key = _get_required_env("SECRET_KEY")
    if secret_key.startswith("replace-") or len(secret_key) < MIN_SECRET_KEY_LENGTH:
        raise RuntimeError(
            "SECRET_KEY must be a non-placeholder value with at least "
            f"{MIN_SECRET_KEY_LENGTH} characters"
        )
    return secret_key


@dataclass(frozen=True)
class Settings:
    database_url: str
    secret_key: str
    access_token_expire_seconds: int
    jwt_issuer: str
    jwt_audience: str
    cors_origins: list[str]
    rate_limit_max_requests: int
    rate_limit_window_seconds: int
    login_rate_limit_max_attempts: int
    login_rate_limit_window_seconds: int
    trust_proxy_headers: bool
    trusted_proxy_cidrs: list[str]
    enable_docs: bool
    max_request_body_bytes: int
    auto_create_tables: bool
    risk_review_amount_threshold: Decimal
    risk_decline_amount_threshold: Decimal
    risk_high_risk_categories: list[str]
    risk_velocity_review_threshold: int
    risk_velocity_window_seconds: int


settings = Settings(
    database_url=_get_required_env("DATABASE_URL"),
    secret_key=_get_secret_key(),
    access_token_expire_seconds=_get_positive_int_env("ACCESS_TOKEN_EXPIRE_SECONDS", 1800),
    jwt_issuer=os.getenv("JWT_ISSUER", "clearance-api"),
    jwt_audience=os.getenv("JWT_AUDIENCE", "clearance-clients"),
    cors_origins=_get_csv_env("CORS_ORIGINS"),
    rate_limit_max_requests=_get_positive_int_env("RATE_LIMIT_MAX_REQUESTS", 120),
    rate_limit_window_seconds=_get_positive_int_env("RATE_LIMIT_WINDOW_SECONDS", 60),
    login_rate_limit_max_attempts=_get_positive_int_env("LOGIN_RATE_LIMIT_MAX_ATTEMPTS", 5),
    login_rate_limit_window_seconds=_get_positive_int_env("LOGIN_RATE_LIMIT_WINDOW_SECONDS", 300),
    trust_proxy_headers=_get_bool_env("TRUST_PROXY_HEADERS", False),
    trusted_proxy_cidrs=_get_csv_env("TRUSTED_PROXY_CIDRS"),
    enable_docs=_get_bool_env("ENABLE_DOCS", False),
    max_request_body_bytes=_get_positive_int_env("MAX_REQUEST_BODY_BYTES", 1_048_576),
    auto_create_tables=_get_bool_env("AUTO_CREATE_TABLES", False),
    risk_review_amount_threshold=_get_positive_decimal_env(
        "RISK_REVIEW_AMOUNT_THRESHOLD",
        "5000.00",
    ),
    risk_decline_amount_threshold=_get_positive_decimal_env(
        "RISK_DECLINE_AMOUNT_THRESHOLD",
        "10000.00",
    ),
    risk_high_risk_categories=_get_csv_env("RISK_HIGH_RISK_CATEGORIES")
    or ["crypto", "gambling", "wire_transfer"],
    risk_velocity_review_threshold=_get_positive_int_env("RISK_VELOCITY_REVIEW_THRESHOLD", 5),
    risk_velocity_window_seconds=_get_positive_int_env("RISK_VELOCITY_WINDOW_SECONDS", 60),
)
