import os

from dotenv import load_dotenv
from sqlalchemy import create_engine
from sqlalchemy.orm import DeclarativeBase, sessionmaker

load_dotenv()

DATABASE_URL = os.environ["DATABASE_URL"]

engine = create_engine(DATABASE_URL)

SessionLocal = sessionmaker(
    # turned both of these off and honestly had to google what they even do.
    # autoflush=True kept sending my half-finished objects to the db before i was ready which freaked me out.
    # off = nothing hits the db until i explicitly .commit(). way less spooky, i stay in control
    autocommit = False,
    autoflush = False,
    bind=engine,
)

class Base(DeclarativeBase):
    pass