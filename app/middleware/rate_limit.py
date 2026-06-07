import ipaddress
import time
from collections import defaultdict, deque
from collections.abc import Callable

from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response


class RateLimitMiddleware(BaseHTTPMiddleware):
    def __init__(
        self,
        app,
        *,
        max_requests: int,
        window_seconds: int,
        trust_proxy_headers: bool = False,
        trusted_proxy_cidrs: list[str] | None = None,
        excluded_paths: set[str] | None = None,
    ):
        super().__init__(app)
        self.max_requests = max_requests
        self.window_seconds = window_seconds
        self.trust_proxy_headers = trust_proxy_headers
        self.trusted_proxy_networks = [
            ipaddress.ip_network(cidr)
            for cidr in trusted_proxy_cidrs or []
        ]
        self.excluded_paths = excluded_paths or set()
        self.requests: dict[str, deque[float]] = defaultdict(deque)
        self.last_cleanup = time.monotonic()

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        if request.url.path in self.excluded_paths:
            return await call_next(request)

        key = self._client_key(request)
        now = time.monotonic()
        self._cleanup(now)
        bucket = self.requests[key]

        while bucket and now - bucket[0] > self.window_seconds:
            bucket.popleft()

        if len(bucket) >= self.max_requests:
            return JSONResponse(
                status_code=429,
                content={
                    "detail": "Rate limit exceeded",
                    "limit": self.max_requests,
                    "window_seconds": self.window_seconds,
                },
                headers={
                    "X-RateLimit-Limit": str(self.max_requests),
                    "X-RateLimit-Remaining": "0",
                },
            )

        bucket.append(now)
        response = await call_next(request)
        response.headers["X-RateLimit-Limit"] = str(self.max_requests)
        response.headers["X-RateLimit-Remaining"] = str(max(0, self.max_requests - len(bucket)))
        return response

    def _cleanup(self, now: float) -> None:
        if now - self.last_cleanup < self.window_seconds:
            return

        for key, bucket in list(self.requests.items()):
            while bucket and now - bucket[0] > self.window_seconds:
                bucket.popleft()
            if not bucket:
                del self.requests[key]

        self.last_cleanup = now

    def _client_key(self, request: Request) -> str:
        forwarded_for = request.headers.get("x-forwarded-for")
        if self._can_trust_forwarded_for(request) and forwarded_for:
            return forwarded_for.split(",")[0].strip()
        if request.client:
            return request.client.host
        return "unknown"

    def _can_trust_forwarded_for(self, request: Request) -> bool:
        if not self.trust_proxy_headers or not self.trusted_proxy_networks or not request.client:
            return False

        try:
            client_ip = ipaddress.ip_address(request.client.host)
        except ValueError:
            return False

        return any(client_ip in network for network in self.trusted_proxy_networks)
