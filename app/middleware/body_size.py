from starlette.responses import JSONResponse, Response
from starlette.types import ASGIApp, Message, Receive, Scope, Send


class RequestBodyTooLarge(Exception):
    pass


class BodySizeLimitMiddleware:
    def __init__(self, app: ASGIApp, *, max_body_bytes: int):
        self.app = app
        self.max_body_bytes = max_body_bytes

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        content_length = self._content_length(scope)
        if content_length:
            try:
                body_size = int(content_length)
            except ValueError:
                await self._send_error(scope, send, 400, "Invalid Content-Length header")
                return

            if body_size > self.max_body_bytes:
                await self._send_error(scope, send, 413, "Request body is too large")
                return

        bytes_seen = 0

        async def limited_receive() -> Message:
            nonlocal bytes_seen

            message = await receive()
            if message["type"] != "http.request":
                return message

            body = message.get("body", b"")
            bytes_seen += len(body)
            if bytes_seen > self.max_body_bytes:
                raise RequestBodyTooLarge()

            return message

        try:
            await self.app(scope, limited_receive, send)
        except RequestBodyTooLarge:
            await self._send_error(scope, send, 413, "Request body is too large")

    def _content_length(self, scope: Scope) -> str | None:
        for name, value in scope.get("headers", []):
            if name == b"content-length":
                return value.decode("latin-1")
        return None

    async def _send_error(
        self,
        scope: Scope,
        send: Send,
        status_code: int,
        detail: str,
    ) -> None:
        response = JSONResponse(
            status_code=status_code,
            content={"detail": detail},
        )
        await response(scope, self._empty_receive, send)

    async def _empty_receive(self) -> Message:
        return {"type": "http.request", "body": b"", "more_body": False}
