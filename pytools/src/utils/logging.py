import sys
from pathlib import Path
from typing import Any

from loguru import logger

logger.remove()


def setup_logger(
    level: str = "INFO",
    format_str: str = "<green>{time:YYYY-MM-DD HH:mm:ss.SSS}</green> | <level>{level: <8}</level> | <cyan>{name}:{function}:{line}</cyan> - <level>{message}</level>",
    console_output: bool = True,
    log_file_path: Path | None = None,
    serialize: bool = False,
    **kwargs: Any,
) -> None:
    """
    Setup the logger.

    Args:
        level (str, optional): The log level. Defaults to "INFO".
        format_str (str, optional): The format of the log message with color tags.
        console_output (bool, optional): Whether to output to console. Defaults to True.
        file_output (bool, optional): Whether to output to file. Defaults to False.
        log_file_path (Path | None): The path to the log file. Defaults to None.
        serialize (bool, optional): Whether to serialize the log. Defaults to False.
        **kwargs: Additional keyword arguments to pass to the logger.
    """
    if console_output:
        logger.add(
            sys.stderr,
            level=level.upper(),
            format=format_str,
            colorize=True,
            serialize=serialize,
            **kwargs,
        )
    if log_file_path:
        log_file_path.parent.mkdir(parents=True, exist_ok=True)
        logger.add(
            log_file_path,
            level=level.upper(),
            format=format_str,
            encoding="utf-8",
            serialize=serialize,
            **kwargs,
        )


def get_logger(name: str) -> Any:
    """
    Get a logger with the given name.
    """
    return logger.bind(name=name)
