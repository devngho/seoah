import asyncio
from asyncio import Lock
from collections.abc import AsyncGenerator
from concurrent.futures import ThreadPoolExecutor
from functools import partial
from io import BytesIO
from typing import cast

import numpy as np
import soundfile as sf
import torch
from omnivoice import OmniVoice

from seoah.config import load_config

# Load the model
_model: OmniVoice | None = None
_model_mutex = Lock()

async def load_omnivoice_model() -> OmniVoice:
    global _model

    config = load_config()

    async with _model_mutex:
        if _model is None or _model.device != torch.device(config.torch_device):
            print(f"Loading TTS model on {config.torch_device}", flush=True)
            _model = OmniVoice.from_pretrained(
                "k2-fsa/OmniVoice",
                device_map=config.torch_device,
                dtype=torch.bfloat16,
            )
            _model = torch.compile(_model, mode="max-autotune", fullgraph=True, dynamic=True)

        return cast(OmniVoice, _model)


async def generate_tts(text: str) -> list[np.ndarray]:
    config = load_config()

    print(f"TTS received text: {text!r}", flush=True)
    model = await load_omnivoice_model()

    print(f"Generating TTS on {model.device} for text: {text}", flush=True)
    return model.generate(text=text, instruct=config.audio_prompt, num_step=8)


async def convert_to_audio(
        generator: AsyncGenerator[str, None],
) -> AsyncGenerator[np.ndarray, None]:
    """Convert text chunks to audio chunks, yielding numpy arrays in input order."""
    async for text_chunk in generator:
        audio_chunks = await generate_tts(text_chunk)
        for audio_chunk in audio_chunks:
            yield audio_chunk


async def convert_to_ogg(
        generator: AsyncGenerator[np.ndarray, None],
) -> AsyncGenerator[bytes, None]:
    """Convert audio chunks to OGG format, yielding bytes in input order."""
    async for audio_chunk in generator:
        # Convert to OGG format using soundfile and bytesio
        with BytesIO() as buffer:
            sf.write(
                buffer, audio_chunk, samplerate=24000, format="OGG", subtype="VORBIS"
            )
            yield buffer.getvalue()
