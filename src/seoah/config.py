import tomllib
from typing import Literal, Callable, Concatenate

from google.genai.types import ThinkingLevel
from pydantic import BaseModel, Field

class ConfigFile(BaseModel):
    api_key: str
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = Field(default="INFO")
    backend_model: str  = Field(default="gemini-3.8-flash")
    thinking_effort: ThinkingLevel = Field(default=ThinkingLevel.LOW)
    prompt: str = Field(default="You are a helpful assistant.")


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

def _load_config() -> ConfigFile:
    with open("config.toml", "rb") as f:
        data = tomllib.load(f)

    return ConfigFile(**data)

def setup_config():
    global config_data, is_initialized
    from seoah.log import log, log_text

    try:
        config_data = _load_config()
    except FileNotFoundError:
        print("config.toml file not found, starting with default values.")
        with open("config.toml", "wb") as file:
            file.write(default_config.encode("utf-8"))

        config_data = _load_config()

    is_initialized = True
    log(lambda: f"Loaded config successfully!", "INFO")
    log(lambda: f"full config: {config_data}", "DEBUG")
