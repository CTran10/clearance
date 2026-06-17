import time
from collections import defaultdict, deque
from threading import Lock

from fastapi import Request


class LoginAttemptLimiter:
    def __init__(self, *, max_attempts: int, window_seconds: int):
        self.max_attempts = max_attempts
        self.window_seconds = window_seconds
        self.failed_attempts: dict[str, deque[float]] = defaultdict(deque)
        self.last_cleanup = time.monotonic()
        # this dict is shared by every request thread at once. without the lock, two threads can read
        # "3 attempts", both go "cool, under the limit", both append, and now there's 5 and the limit did nothing.
        # the lock makes check-then-update one indivisible move. classic race i would NOT have seen coming
        self.lock = Lock()

    def is_limited(self, request: Request, email: str) -> bool:
        key = self._key(request, email)
        now = time.monotonic()
        with self.lock:
            self._cleanup(now)
            bucket = self.failed_attempts.get(key)
            if bucket is None:
                return False
            self._remove_expired_attempts(bucket, now)
            if not bucket:
                del self.failed_attempts[key]
                return False
            return len(bucket) >= self.max_attempts

    def record_failure(self, request: Request, email: str) -> None:
        key = self._key(request, email)
        now = time.monotonic()
        with self.lock:
            self._cleanup(now)
            bucket = self.failed_attempts[key]
            self._remove_expired_attempts(bucket, now)
            bucket.append(now)

    def clear(self, request: Request, email: str) -> None:
        key = self._key(request, email)
        with self.lock:
            self.failed_attempts.pop(key, None)

    def _remove_expired_attempts(self, bucket: deque[float], now: float) -> None:
        while bucket and now - bucket[0] > self.window_seconds:
            bucket.popleft()

    def _cleanup(self, now: float) -> None:
        if now - self.last_cleanup < self.window_seconds:
            return

        for key, bucket in list(self.failed_attempts.items()):
            self._remove_expired_attempts(bucket, now)
            if not bucket:
                del self.failed_attempts[key]

        self.last_cleanup = now

    def _key(self, request: Request, email: str) -> str:
        client_host = request.client.host if request.client else "unknown"
        return f"{client_host}:{email}"
