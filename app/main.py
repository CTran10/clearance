from fastapi import FastAPI
from app.auth.routes import router as auth_router
from app.users.routes import router as users_router

app = FastAPI()
from app.db.session import Base, engine
from app.db import models

Base.metadata.create_all(bind=engine)

app.include_router(auth_router)
app.include_router(users_router)

# Basic health check for confirming the API process is running.
@app.get("/health")
def health():
    return {"status": "ok"}

