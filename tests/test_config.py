import pytest

from app.core.config import _get_bool_env, _get_positive_decimal_env, _get_positive_int_env


def test_positive_int_env_rejects_zero_and_negative_values(monkeypatch):
    monkeypatch.setenv("TEST_INT", "0")

    with pytest.raises(RuntimeError, match="TEST_INT must be greater than 0"):
        _get_positive_int_env("TEST_INT", 1)


def test_positive_int_env_rejects_non_integer_values(monkeypatch):
    monkeypatch.setenv("TEST_INT", "abc")

    with pytest.raises(RuntimeError, match="TEST_INT must be an integer"):
        _get_positive_int_env("TEST_INT", 1)


def test_positive_decimal_env_rejects_zero_and_negative_values(monkeypatch):
    monkeypatch.setenv("TEST_DECIMAL", "0")

    with pytest.raises(RuntimeError, match="TEST_DECIMAL must be greater than 0"):
        _get_positive_decimal_env("TEST_DECIMAL", "1.00")


def test_positive_decimal_env_rejects_non_decimal_values(monkeypatch):
    monkeypatch.setenv("TEST_DECIMAL", "not-money")

    with pytest.raises(RuntimeError, match="TEST_DECIMAL must be a decimal value"):
        _get_positive_decimal_env("TEST_DECIMAL", "1.00")


def test_positive_decimal_env_rejects_non_finite_values(monkeypatch):
    monkeypatch.setenv("TEST_DECIMAL", "NaN")

    with pytest.raises(RuntimeError, match="TEST_DECIMAL must be a decimal value"):
        _get_positive_decimal_env("TEST_DECIMAL", "1.00")


def test_bool_env_rejects_unknown_values(monkeypatch):
    monkeypatch.setenv("TEST_BOOL", "sometimes")

    with pytest.raises(RuntimeError, match="TEST_BOOL must be a boolean value"):
        _get_bool_env("TEST_BOOL")
