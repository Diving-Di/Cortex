"""Compatibility import for the new core configuration module."""

from .core.config import BASE_DIR, Settings, get_config, get_settings

__all__ = ["BASE_DIR", "Settings", "get_config", "get_settings"]
