from fastapi import Header, status, HTTPException
from app.core.security import decode_access_token
from app.data.users import users

#vaidate current user's session and turn into a real valid user obj

def get_current_user(authorization: str | None = Header(default=None)) -> dict:
    if not authorization:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Missing Authorization header"
        )

    # the header looks like "Bearer eyJhbGci...". partition(" ") splits it into 3 pieces
    # at the FIRST space: scheme, the space itself (the "_" we throw away), and the token.
    # i tried .split(" ") first but that explodes if there's a weird extra space, partition just chills
    scheme, _, token = authorization.partition(" ")

    if scheme.lower() != "bearer" or not token:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="Invalid Authorization header"
        )

    payload = decode_access_token(token)
    user_id = int(payload["sub"])

    user = next((user for user in users if user["id"] == user_id), None)

    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User no longer exists"
        )

    return user