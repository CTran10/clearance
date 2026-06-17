from fastapi import APIRouter, Depends, HTTPException, Request, status
from fastapi.responses import JSONResponse
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.auth.rate_limit import LoginAttemptLimiter
from app.auth.schemas import (
    LoginRequest,
    LoginResponse,
    RegisterRequest,
    RegisterResponse,
)
from app.auth.service import create_user, find_user_by_email, password_matches
from app.core.config import settings
from app.core.request_context import get_request_id
from app.core.security import create_access_token
from app.db.dependencies import get_db

router = APIRouter(prefix="/auth", tags=["auth"])
login_attempt_limiter = LoginAttemptLimiter(
    max_attempts=settings.login_rate_limit_max_attempts,
    window_seconds=settings.login_rate_limit_window_seconds,
)


@router.post(
    "/register",
    status_code=status.HTTP_201_CREATED,
    response_model=RegisterResponse,
)
def register(
    payload: RegisterRequest,
    request: Request,
    db: Session = Depends(get_db),
):
    # shoved all the messy db poking into service helpers — a route should read like a sentence, not a db tutorial.
    # this pre-check is just the "nice error" path; the REAL duplicate guard is the IntegrityError catch below
    # (two people can register the same email in the same millisecond and both pass this check — db constraint is the only true referee)
    if find_user_by_email(db, payload.email):
        raise_email_registered()

    try:
        user = create_user(db, email=payload.email, password=payload.password)
        create_audit_event(
            db,
            action="REGISTERED_USER",
            entity_type="user",
            entity_id=user.id,
            user_id=user.id,
            request_id=get_request_id(request),
        )
        db.commit()
    except IntegrityError:
        db.rollback()
        raise_email_registered()

    db.refresh(user)
    return {
        "message": "User Registered",
        "user": {
            "id": user.id,
            "email": user.email,
        },
    }


@router.post("/login", response_model=LoginResponse)
def login(
    payload: LoginRequest,
    request: Request,
    db: Session = Depends(get_db),
):
    # rate limit BEFORE we even look at the password. otherwise someone can just brute-force passwords forever.
    # keyed on ip+email so one attacker can't lock out a victim by spamming THEIR email (learned that's a real attack)
    if login_attempt_limiter.is_limited(request, payload.email):
        return login_rate_limited_response()

    # generic 401 on purpose: same exact message whether the email doesn't exist OR the password is wrong.
    # saying "no such email" would quietly leak which emails are registered (user enumeration). felt unhelpful, is the point.
    # bonus: password_matches still runs a hash check even on a missing user so the response time doesn't rat us out either
    user = find_user_by_email(db, payload.email)
    if not password_matches(user, payload.password):
        login_attempt_limiter.record_failure(request, payload.email)
        raise_invalid_credentials()

    login_attempt_limiter.clear(request, payload.email)
    access_token = create_access_token(user.id)
    create_audit_event(
        db,
        action="LOGGED_IN",
        entity_type="user",
        entity_id=user.id,
        user_id=user.id,
        request_id=get_request_id(request),
    )
    db.commit()

    return {
        "message": "Login Successful",
        "access_token": access_token,
        "token_type": "bearer",
        "user": {
            "id": user.id,
            "email": user.email,
        },
    }


def raise_email_registered() -> None:
    raise HTTPException(
        status_code=status.HTTP_409_CONFLICT,
        detail="Email is already registered",
    )


def raise_invalid_credentials() -> None:
    raise HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="Invalid email or password",
    )


def login_rate_limited_response() -> JSONResponse:
    return JSONResponse(
        status_code=status.HTTP_429_TOO_MANY_REQUESTS,
        content={
            "detail": "Too many failed login attempts",
            "limit": login_attempt_limiter.max_attempts,
            "window_seconds": login_attempt_limiter.window_seconds,
        },
    )
