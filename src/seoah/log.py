from typing import Literal, Callable

from seoah.config import load_config


def log_text(message: str, level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = "INFO") -> None:
    """
    Log a message with the specified log level.

    :param message: The message to log.
    :param level: The log level, which can be "DEBUG", "INFO", "WARNING", or "ERROR". Defaults to "INFO".
    :return: None
    """

    log(lambda: message, level)


def log(message: Callable[[], str], level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = "INFO") -> None:
    """
    Log a message with the specified log level.

    :param message: A callable that returns the message to log. This is suitable for logs that are expensive to compute.
    :param level: The log level, which can be "DEBUG", "INFO", "WARNING", or "ERROR". Defaults to "INFO".
    :return: None
    """
    config = load_config()

    if level == "DEBUG" and config.log_level != "DEBUG":
        return
    if level == "INFO" and config.log_level not in ["DEBUG", "INFO"]:
        return

    print(f"[{level}] {message()}")
