import time

from fastapi import HTTPException, status
from jose import jwt, JWTError
from passlib.context import CryptContext

from app.core.config import settings

ACCESS_TOKEN_EXPIRE_SECONDS = settings.access_token_expire_seconds
# spent way too long thinking HS256 vs RS256 was about how strong the hash is lol.
# turns out HS = one shared secret, RS = public/private key pair. we're tiny so shared secret is fine for now
ALGORITHM = "HS256"
SECRET_KEY = settings.secret_key  # pulled from settings now instead of os.getenv everywhere — one place to forget to set it instead of five

# added bcrypt_sha256 in front of plain bcrypt bc bcrypt silently TRUNCATES passwords past 72 bytes (!!).
# so two different long passwords could match. sha256-ing first squishes any length down to a safe size. sneaky bug avoided
pwd_context = CryptContext(schemes=["bcrypt_sha256", "bcrypt"], deprecated="auto")


def hash_password(password: str) -> str:
    return pwd_context.hash(password)


def verify_password(plain_password: str, password_hash: str) -> bool:
    return pwd_context.verify(plain_password, password_hash)


def create_access_token(user_id: int) -> str:
    now = int(time.time())

    payload = {
        "sub": str(user_id),  # "sub" = subject. confusingly NOT something you subscribe to. it's just "who is this"
        "iat": now,           # issued-at. learned the hard way these are unix seconds not millis (my tokens were "valid" til the year 56000)
        "exp": now + ACCESS_TOKEN_EXPIRE_SECONDS,
    }

    return jwt.encode(payload, SECRET_KEY, algorithm=ALGORITHM)


def decode_access_token(token: str) -> dict:
    try:
        return jwt.decode(token, SECRET_KEY, algorithms=[ALGORITHM])
    except JWTError:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid access token",
        )
