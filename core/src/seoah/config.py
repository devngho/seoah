from typing import Literal

import tomllib
from google.genai.types import ThinkingLevel
from pydantic import BaseModel, Field, HttpUrl, SecretStr


class ConfigFile(BaseModel):
    api_key: str
    discord_api_key: str | None = Field(default=None)

    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = Field(default="INFO")
    backend_model: str = Field(default="gemini-3.8-flash")
    thinking_effort: ThinkingLevel = Field(default=ThinkingLevel.LOW)
    prompt: str = Field(default="You are a helpful assistant.")
    audio_prompt: str = Field(default="female, korean accent, teenager, high pitch")

    debounce_ms: int = Field(
        default=2500,
        description="Debounce time in milliseconds for Discord bot responses.",
    )

    torch_device: str = Field(
        default="cpu", description="Torch device for model inference."
    )

    tts_backend: Literal["cosyvoice_cpp", "cosyvoice", "supertone", "omnivoice"] = (
        "cosyvoice_cpp"
    )
    # None starts a private local server; a URL connects to an existing server.
    cosyvoice_cpp_url: HttpUrl | None = None
    cosyvoice_cpp_api_key: SecretStr | None = None
    cosyvoice_cpp_repo: str = "~/다운로드/cosyvoice.cpp"
    cosyvoice_cpp_binary: str = "build-vulkan/bin/cosyvoice-server"
    cosyvoice_cpp_backend_path: str = "build-vulkan/lib"
    cosyvoice_cpp_backend: str = "Vulkan0"
    cosyvoice_cpp_model: str = "experiments/audio_clone6-v3/model-q8.gguf"
    cosyvoice_cpp_prompt: str = "experiments/audio_clone6-v3/prompt.gguf"
    cosyvoice_cpp_model_name: str = "seoah-v3"
    cosyvoice_cpp_voice: str = "seoah"
    cosyvoice_cpp_threads: int = Field(default=16, ge=1)
    cosyvoice_cpp_startup_timeout: float = Field(default=120, gt=0)
    cosyvoice_cpp_timeout: float = Field(default=180, gt=0)

    cosyvoice_repo: str = "~/다운로드/CosyVoice"
    # Relative paths below are resolved against cosyvoice_repo.
    cosyvoice_python: str = ".venv/bin/python"
    cosyvoice_model: str = "exp/audio_clone6_300m/run1/model"
    cosyvoice_prompt_wav: str = (
        "~/PycharmProjects/seoah/voice-trainer/audio_clone6/00001.wav"
    )
    # Plain transcript; the worker adds the instruction prefix only for v3.
    cosyvoice_prompt_text: str = "오늘 점심은 그냥 편의점에서 때웠어요. 귀찮아서."
    cosyvoice_threads: int = Field(default=16, ge=1)
    cosyvoice_timeout: float = Field(default=600, gt=0)


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
    try:
        with open(path, "rb") as f:
            data = tomllib.load(f)
    except (OSError, tomllib.TOMLDecodeError) as error:
        error.add_note(f"While loading configuration from {path}")
        raise  # Preserve FileNotFoundError for setup_config's default-file handling.

    return ConfigFile(**data)


def setup_config(path: str | None = None) -> None:
    global config_data, is_initialized
    from seoah.log import log

    path = path or "../config.toml"

    try:
        config_data = _load_config(path)
    except FileNotFoundError:
        print("config file not found, starting with default values.")
        with open(path, "wb") as file:
            file.write(default_config.encode("utf-8"))

        config_data = _load_config(path)

    is_initialized = True
    log(lambda: "Loaded config successfully!", "INFO")
    log(lambda: f"full config: {config_data}", "DEBUG")
