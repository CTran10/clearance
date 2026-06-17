from collections.abc import Generator

from sqlalchemy.orm import Session

from app.db.session import SessionLocal


# this `yield` thing broke my brain for a sec. it's not a normal return —
# fastapi runs the route WHERE the yield is, then comes BACK here after to do the finally.
# so the db session opens, the endpoint borrows it, and it always closes even if stuff blows up. kinda elegant ngl
def get_db() -> Generator[Session, None, None]:
    db = SessionLocal()
    try:
        yield db
    finally:
        db.close()
