from fastapi import Request


def get_request_id(request: Request) -> str | None:
    return getattr(request.state, "request_id", None)
