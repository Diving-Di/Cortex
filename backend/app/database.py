"""Compatibility import for database dependencies used by legacy routers."""
from .core.database import Base, SessionLocal, engine, get_db

__all__ = ["Base", "SessionLocal", "engine", "get_db"]
