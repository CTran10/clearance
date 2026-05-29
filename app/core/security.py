from passlib.context import CryptContext
import os
import time
from jose import jwt, JWTError
from fastapi import HTTPException, status


ACCESS_TOKEN_EXPIRE_SECONDS = int(os.getenv("ACCESS_TOKEN_EXPIRE_SECONDS", "1800"))
SECRET_KEY = os.getenv("SECRET_KEY", "dev-only-change-me")
ALGORITHM = "HS256" #common, supported, fast, secure enough for a simple app, but trades off hash strengths and depends on secret key quality

pwd_context = CryptContext(schemes=["bcrypt"], deprecated="auto")

# input user provided password and use .hash from cryptcontext to hash the password
def hash_password(password: str) -> str:
    return pwd_context.hash(password)

#compareds user input against stored pw hash
def verify_password(plain_password: str, password_hash:str) -> bool:
    return pwd_context.verify(plain_password,password_hash)

# generate auth token
def create_access_token(user_id: int) -> str:
    now = int(time.time())

    payload = {
        "sub": str(user_id),
        "iat": now,
        "exp": now + ACCESS_TOKEN_EXPIRE_SECONDS,
    }

    return jwt.encode(payload, SECRET_KEY, algorithm=ALGORITHM)

#decodes token using env secret key and alg type
def decode_access_token(token: str) -> dict:
    try:
        payload = jwt.decode(token, SECRET_KEY, algorithms=[ALGORITHM])
        return payload
    except JWTError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid access token"
        )
