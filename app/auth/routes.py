from fastapi import APIRouter, Depends, HTTPException, Request, status
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session

from app.audit.service import create_audit_event
from app.auth.schemas import (
    LoginRequest,
    LoginResponse,
    RegisterRequest,
    RegisterResponse,
)
from app.auth.service import create_user, find_user_by_email, password_matches
from app.core.request_context import get_request_id
from app.core.security import create_access_token
from app.db.dependencies import get_db

router = APIRouter(prefix="/auth", tags=["auth"])


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
    user = find_user_by_email(db, payload.email)
    if not password_matches(user, payload.password):
        raise_invalid_credentials()

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
