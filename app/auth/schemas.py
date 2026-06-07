from pydantic import BaseModel, ConfigDict, EmailStr, Field, field_validator

from app.users.schemas import UserResponse


class RegisterResponse(BaseModel):
    message: str
    user: UserResponse


class LoginResponse(BaseModel):
    message: str
    access_token: str
    token_type: str
    user: UserResponse


class RegisterRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    email: EmailStr
    password: str = Field(min_length=8, max_length=128)

    @field_validator("email")
    @classmethod
    def normalize_email_address(cls, value: EmailStr) -> str:
        return normalize_email(value)

    @field_validator("password")
    @classmethod
    def password_must_have_special_character(cls, value):
        special_characters = "!@#$%^&*()-_=+[]{}|;:'\",.<>?/`~"
        if not any(char in special_characters for char in value):
            raise ValueError("Password must contain at least one special character")
        return value

    @field_validator("password")
    @classmethod
    def password_must_have_number(cls, value):
        numbers = "1234567890"
        if not any(char in numbers for char in value):
            raise ValueError("Password must contain at least one number")
        return value


class LoginRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    email: EmailStr
    password: str = Field(min_length=1, max_length=128)

    @field_validator("email")
    @classmethod
    def normalize_email_address(cls, value: EmailStr) -> str:
        return normalize_email(value)


def normalize_email(value: EmailStr) -> str:
    return str(value).strip().lower()
