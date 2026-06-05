"""Filter expression language tests."""
import pytest
from quark_client import apply_filter, eval_expr


def test_basic_comparison():
    items = [{"stars": 50}, {"stars": 200}, {"stars": 1000}]
    assert len(apply_filter(items, "stars > 100")) == 2


def test_and():
    items = [
        {"name": "a", "stars": 50, "owner": "x"},
        {"name": "b", "stars": 200, "owner": "x"},
        {"name": "c", "stars": 200, "owner": "y"},
    ]
    result = apply_filter(items, "stars > 100 && owner == 'x'")
    assert len(result) == 1
    assert result[0]["name"] == "b"


def test_or_parens():
    items = [
        {"stars": 200, "verified": True},
        {"stars": 50, "verified": False},
    ]
    assert len(apply_filter(items, "(stars > 100 || verified == true)")) == 1


def test_not():
    items = [{"archived": True}, {"archived": False}]
    assert len(apply_filter(items, "!archived")) == 1


def test_in():
    items = [{"lang": "go"}, {"lang": "rust"}, {"lang": "python"}]
    assert len(apply_filter(items, "lang in ['go', 'rust']")) == 2


def test_notin():
    items = [{"status": "active"}, {"status": "archived"}, {"status": "deleted"}]
    assert len(apply_filter(items, "status notIn ['archived', 'deleted']")) == 1


def test_matches():
    items = [{"email": "a@example.com"}, {"email": "b@other.com"}]
    assert len(apply_filter(items, "email matches '.*@example.*'")) == 1


def test_contains():
    items = [{"name": "claude-code"}, {"name": "mcp"}]
    assert len(apply_filter(items, "name contains 'claude'")) == 1


def test_starts_with():
    items = [{"name": "AI Assistant"}, {"name": "Database"}]
    assert len(apply_filter(items, "name startsWith 'AI'")) == 1


def test_nested_field():
    items = [
        {"meta": {"score": 50}},
        {"meta": {"score": 200}},
    ]
    assert len(apply_filter(items, "meta.score > 100")) == 1


def test_arithmetic():
    items = [{"a": 10, "b": 5}, {"a": 3, "b": 5}]
    assert len(apply_filter(items, "a > b * 1.5")) == 1


def test_eval_expr():
    assert eval_expr({"stars": 100}, "stars > 50") is True
    assert eval_expr({"stars": 30}, "stars > 50") is False
