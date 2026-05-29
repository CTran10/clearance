from fastapi import APIRouter, HTTPException, status, Depends
from sqlalchemy.orm import Session

from app.db.dependencies import get_db
from app.db.models import User

# TODO: Replace wildcard imports with explicit imports 
from app.auth.schemas import *
from app.core.security import *

#creates a mini route group, so we]an attach related endpoints to a router instead of attaching every endpoint to main
router = APIRouter(prefix="/auth", tags = ["auth"])
@router.post(
        "/register", #route
        status_code=status.HTTP_201_CREATED, #successful response status code
        response_model=RegisterResponse, #dictates exact response schema 
        )
def register(payload: RegisterRequest, db: Session = Depends(get_db)):
    #Check if user exists
    #fixed O(n) lookup using unique key in db 
    existing_user = db.query(User).filter(User.email == payload.email).first()
    #if user exists raise 409 conflict
    if existing_user: 
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Email is already registered"
        )
    user = User(
       email=payload.email,
       password_hash=hash_password(payload.password),
   )
    db.add(user)
    db.commit()
    db.refresh(user)
    return {
        "message": "User Registered",
        "user": {
            "id": user.id,
            "email": user.email
        }
    }


#login 
@router.post("/login", response_model=LoginResponse)
def login(payload: LoginRequest, db: Session = Depends(get_db)):
    # Look up the user and reject invalid credentials with a generic 401.
    # ok so originally i had "email not found" vs "wrong password" as two diff messages bc it felt helpful.
    # apparently that's an oopsie — lets an attacker figure out which emails are real (user enumeration).
    # so now both say the exact same thing. felt unhelpful, is actually the point
    user = db.query(User).filter(User.email == payload.email).first()
    if not user:
        raise HTTPException(
            status_code = status.HTTP_401_UNAUTHORIZED,
            detail = "Invalid email or password"
        )
    if not verify_password(payload.password, user.password_hash):
        raise HTTPException(
            status_code = status.HTTP_401_UNAUTHORIZED,
            detail = "Invalid email or password"
        )
    # Create jwt token
    access_token = create_access_token(user.id)
    
    return {
        "message": "Login Successful",
        "access_token": access_token,
        "token_type": "bearer",
        "user": {
            "id": user.id,
            "email": user.email
        }
    }
