from pydantic import BaseModel, EmailStr, field_validator, Field
#create response schemas for validatiuon, consistency, and filtering
class UserResponse(BaseModel):
    id: int
    email: EmailStr

class RegisterResponse(BaseModel):
    message: str
    user: UserResponse

class LoginResponse(BaseModel):
    message: str
    access_token: str
    token_type: str
    user: UserResponse

class MeResponse(BaseModel):
    user: UserResponse


class RegisterRequest(BaseModel):
    email: EmailStr
    password: str = Field(min_length = 8)

    @field_validator("password")
    @classmethod
    def password_must_have_special_character(cls, value):
        special_characters = "!@#$%^&*()-_=+[]{}|;:'\",.<>?/`~"
        if not any(char in special_characters for char in value):
            raise ValueError("Password must contain at least one special character")
        return value
    @field_validator("password")
    @classmethod
    def password_must_have_number(cls,value):
        numbers = "1234567890"
        if not any(char in numbers for char in value):
            raise ValueError("Password must contain at least one number")
        return value

class LoginRequest(BaseModel):
    email: EmailStr
    password: str