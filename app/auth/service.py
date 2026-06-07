from sqlalchemy.orm import Session

from app.core.security import hash_password, verify_password
from app.db.models import User

DUMMY_PASSWORD_HASH = (
    "$bcrypt-sha256$v=2,t=2b,r=12$SM38yB/mbPTDX39aqr426O"
    "$LqC.N40CxJXLdNtmkGYQXs.fONeOXF6"
)


def find_user_by_email(db: Session, email: str) -> User | None:
    return db.query(User).filter(User.email == email).first()


def create_user(db: Session, *, email: str, password: str) -> User:
    user = User(
        email=email,
        password_hash=hash_password(password),
    )
    db.add(user)
    db.flush()
    return user


def password_matches(user: User | None, password: str) -> bool:
    if not user:
        # this looks insane — why hash-check a password for a user that doesn't exist?? TIMING ATTACKS.
        # if we returned False instantly for unknown emails, an attacker could measure response time
        # and tell "real email + wrong pw" (slow, did a real bcrypt check) apart from "fake email" (instant).
        # so we burn the same ~milliseconds on a fake hash. wild that "wasting time on purpose" is the secure move
        verify_password(password, DUMMY_PASSWORD_HASH)
        return False

    return verify_password(password, user.password_hash)
