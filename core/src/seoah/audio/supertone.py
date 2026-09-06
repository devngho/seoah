from io import BytesIO
from typing import AsyncGenerator

import numpy as np
from supertonic import TTS
import soundfile as sf

from seoah.config import load_config

tts = TTS(auto_download=True)
style = tts.get_voice_style(voice_name="F2")


async def generate_supertone_tts(text: str) -> np.ndarray:
    config = load_config()

    wav, duration = tts.synthesize(text, voice_style=style, lang="ko", verbose=True)

    return wav


async def convert_to_audio(
        generator: AsyncGenerator[str, None],
) -> AsyncGenerator[np.ndarray, None]:
    """Convert text chunks to audio chunks, yielding numpy arrays in input order."""
    async for text_chunk in generator:
        audio_chunk = await generate_supertone_tts(text_chunk)
        yield audio_chunk.reshape(-1, 1)  # (1, n) -> (n, 1)


async def convert_to_ogg(
        generator: AsyncGenerator[np.ndarray, None],
) -> AsyncGenerator[bytes, None]:
    """Convert audio chunks to OGG format, yielding bytes in input order."""
    async for audio_chunk in generator:
        # Convert to OGG format using soundfile and bytesio
        with BytesIO() as buffer:
            sf.write(
                buffer, audio_chunk, samplerate=44100, format="OGG", subtype="VORBIS"
            )
            yield buffer.getvalue()
