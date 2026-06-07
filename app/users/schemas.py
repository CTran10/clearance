from pydantic import BaseModel, EmailStr


class UserResponse(BaseModel):
    id: int
    email: EmailStr


class MeResponse(BaseModel):
    user: UserResponse
