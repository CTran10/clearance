from fastapi import Header, status, HTTPException, Depends
from app.core.security import decode_access_token
from sqlalchemy.orm import Session
from app.db.dependencies import get_db
from app.db.models import User

#vaidate current user's session and turn into a real valid user obj

def get_current_user(authorization: str | None = Header(default=None),
                     db: Session = Depends(get_db),
                     ) -> User:
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

    user = db.query(User).filter(User.id == user_id).first()

    if not user:
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="User no longer exists"
        )

    return user