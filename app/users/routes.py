from fastapi import APIRouter, Depends
from app.auth.schemas import MeResponse
from app.users.dependencies import get_current_user
from app.db.models import User

router = APIRouter(prefix="/users", tags = ["users"])

#gets current user's information after they're authorized 
@router.get("/me", response_model = MeResponse)
def get_me(current_user: User = Depends(get_current_user)):
    return {
        "user": {
            "id": current_user.id,
            "email": current_user.email
        }
    }