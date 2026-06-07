from fastapi import APIRouter, Depends

from app.db.models import User
from app.users.dependencies import get_current_user
from app.users.schemas import MeResponse

router = APIRouter(prefix="/users", tags=["users"])


@router.get("/me", response_model=MeResponse)
def get_me(current_user: User = Depends(get_current_user)):
    return {
        "user": {
            "id": current_user.id,
            "email": current_user.email,
        },
    }
