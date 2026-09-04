import tomllib
from typing import Literal, Callable, Concatenate, Optional

from google.genai.types import ThinkingLevel
from pydantic import BaseModel, Field

class ConfigFile(BaseModel):
    api_key: str
    discord_api_key: Optional[str] = Field(default=None)

    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = Field(default="INFO")
    backend_model: str  = Field(default="gemini-3.8-flash")
    thinking_effort: ThinkingLevel = Field(default=ThinkingLevel.LOW)
    prompt: str = Field(default="You are a helpful assistant.")

    debounce_ms: int = Field(default=2500, description="Debounce time in milliseconds for Discord bot responses.")


default_config = """api_key = "[put your api key here]"
backend_model = "gemini-3.8-flash"
log_level = "INFO"
"""

config_data: ConfigFile = ConfigFile(api_key="loading")
is_initialized = False

def load_config() -> ConfigFile:
    if not is_initialized:
        setup_config()

    return config_data

def _load_config(path: str) -> ConfigFile:
    with open(path, "rb") as f:
        data = tomllib.load(f)

    return ConfigFile(**data)

def setup_config(path: Optional[str] = None) -> None:
    global config_data, is_initialized
    from seoah.log import log, log_text

    path = path or "../config.toml"

    try:
        config_data = _load_config(path)
    except FileNotFoundError:
        print("config file not found, starting with default values.")
        with open(path, "wb") as file:
            file.write(default_config.encode("utf-8"))

        config_data = _load_config(path)

    is_initialized = True
    log(lambda: f"Loaded config successfully!", "INFO")
    log(lambda: f"full config: {config_data}", "DEBUG")
