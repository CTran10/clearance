from sqlalchemy import create_engine
from sqlalchemy.orm import DeclarativeBase, sessionmaker

from app.core.config import settings


def create_database_engine(database_url: str):
    connect_args = {}
    if database_url.startswith("sqlite"):
        connect_args["check_same_thread"] = False

    return create_engine(
        database_url,
        connect_args=connect_args,
        # pool_pre_ping = "knock on the connection before using it." postgres/the cloud will silently kill
        # idle connections and then my first query of the morning would explode with a stale-connection error.
        # this just quietly throws away dead ones instead. fixed a really annoying flaky bug
        pool_pre_ping=True,
    )


engine = create_database_engine(settings.database_url)

SessionLocal = sessionmaker(
    # turned both of these off and honestly had to google what they even do.
    # autoflush=True kept sending my half-finished objects to the db before i was ready which freaked me out.
    # off = nothing hits the db until i explicitly .commit(). way less spooky, i stay in control
    autocommit=False,
    autoflush=False,
    bind=engine,
)


class Base(DeclarativeBase):
    pass
