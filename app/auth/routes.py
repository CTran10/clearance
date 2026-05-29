from fastapi import APIRouter, HTTPException, status, Header
from app.data.users import users
from app.auth.schemas import *
from app.core.security import *

#creates a mini route group, so we]an attach related endpoints to a router instead of attaching every endpoint to main
router = APIRouter(prefix="/auth", tags = ["auth"])
@router.post(
        "/register", #route
        status_code=status.HTTP_201_CREATED, #successful response status code
        response_model=RegisterResponse, #dictates exact response schema 
        )
def register(payload: RegisterRequest):
    #Check if user exists
    #TODO: Fix o(n) user search using hashmap (unique key for emails)
    existing_user = next(
        (user for user in users if user["email"] == payload.email),
        None
    )
    #if user exists raise 409 conflict
    if existing_user: 
        raise HTTPException(
            status_code=status.HTTP_409_CONFLICT,
            detail="Email is already registered"
        )
    #add user to existing user TODO: Create postgres database to store users outside of RAM
    user = {
        "id": len(users) +1,
        "email": payload.email,
        "password_hash": hash_password(payload.password)
    }
    users.append(user)
    return {
        "message": "User Registered",
        "user": {
            "id": user["id"],
            "email": payload.email
        }
    }


#login 
@router.post("/login", response_model=LoginResponse)
def login(payload: LoginRequest):
    # Look up the user and reject invalid credentials with a generic 401.
    # ok so originally i had "email not found" vs "wrong password" as two diff messages bc it felt helpful.
    # apparently that's an oopsie — lets an attacker figure out which emails are real (user enumeration).
    # so now both say the exact same thing. felt unhelpful, is actually the point
    user = next(
        (user for user in users if user["email"] == payload.email),
        None
    )
    if not user:
        raise HTTPException(
            status_code = status.HTTP_401_UNAUTHORIZED,
            detail = "Invalid email or password"
        )
    if not verify_password(payload.password, user["password_hash"]):
        raise HTTPException(
            status_code = status.HTTP_401_UNAUTHORIZED,
            detail = "Invalid email or password"
        )
    # Create jwt token
    access_token = create_access_token(user["id"])
    
    return {
        "message": "Login Successful",
        "access_token": access_token,
        "token_type": "bearer",
        "user": {
            "id": user["id"],
            "email": user["email"]
        }
    }
