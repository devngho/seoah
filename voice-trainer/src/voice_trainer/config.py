from typing import Literal

import tomllib
from google.genai.types import ThinkingLevel
from pydantic import BaseModel, Field


class ConfigFile(BaseModel):
    api_key: str

    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR"] = Field(default="INFO")
    backend_model: str = Field(default="gemini-3.8-flash")
    thinking_effort: ThinkingLevel = Field(default=ThinkingLevel.MEDIUM)
    prompt: str = Field(default="You are a helpful assistant.")
    instruction_for_generation: str = Field(
        default="Generate {count} sentences that is suitable for the character. You MUST NOT include newline characters. Contain at least 20% of the sentences with everyday conversation."
    )

    audio_design_model_kind: Literal["qwen-tts"] = Field(
        default="qwen-tts", description="Kind of audio model to use for TTS."
    )
    audio_design_model: str = Field(default="Qwen/Qwen3-TTS-12Hz-1.7B-VoiceDesign")
    audio_design_sample_count: int = Field(
        default=3, ge=1, description="Maximum unique corpus sentences for voice design."
    )
    audio_design_candidate_count: int = Field(
        default=4,
        ge=1,
        description="Candidate audio samples for each unique corpus sentence for voice design.",
    )
    audio_design_prompt: str = Field(
        default="Speak as a cute and shy girl, speaking sliently and softly but cute tone."
    )
    audio_clone_model_kind: Literal["qwen-tts", "omnivoice"] = Field(
        default="qwen-tts", description="Kind of audio model to use for TTS."
    )
    audio_clone_model: str = Field(default="Qwen/Qwen3-TTS-12Hz-1.7B-Base")
    audio_batch_size: int = Field(
        default=2, ge=1, description="Texts per inference batch."
    )
    audio_lang: str = Field(
        default="Korean", description="Language name for the audio model."
    )
    audio_metadata_format: str = Field(
        default="{path}|KO-seoah|KO|{text}",
        description="Format for the metadata file. Use {path} for the audio file path and {text} for the corresponding text.",
    )
    folder_to_save_audio_design: str = Field(
        default="audio_design", description="Path to save the generated audio."
    )
    folder_to_save_audio_clone: str = Field(
        default="audio_clone", description="Path to save the generated audio."
    )

    num_sentence_to_generate: int = Field(
        default=100, description="Number of sentences to generate for training."
    )
    path_to_save_corpus: str = Field(
        default="corpus.txt", description="Path to save the generated sentences."
    )

    torch_device: str = Field(
        default="cpu", description="Torch device for model inference."
    )


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


def setup_config(path: str | None = None) -> None:
    global config_data, is_initialized
    from voice_trainer.log import log

    path = path or "config.toml"

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
