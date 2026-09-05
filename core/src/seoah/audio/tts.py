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

_MAX_IN_FLIGHT = 4
executor = ThreadPoolExecutor(max_workers=_MAX_IN_FLIGHT, thread_name_prefix="tts")


async def load_model() -> OmniVoice:
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
    model = await load_model()

    print(f"Generating TTS on {model.device} for text: {text}", flush=True)
    return await asyncio.get_running_loop().run_in_executor(
        executor,
        partial(model.generate, text=text, instruct=config.audio_prompt, num_step=8),
    )


async def convert_to_audio(
        generator: AsyncGenerator[str, None],
) -> AsyncGenerator[np.ndarray, None]:
    """Convert up to four chunks concurrently, yielding audio in input order."""

    queue: asyncio.Queue[asyncio.Task[list[np.ndarray]] | None] = asyncio.Queue()
    slots = asyncio.Semaphore(_MAX_IN_FLIGHT)
    pending: set[asyncio.Task] = set()

    async def produce() -> None:
        try:
            async for chunk in generator:
                await slots.acquire()
                task = asyncio.create_task(generate_tts(chunk))
                pending.add(task)
                queue.put_nowait(task)
        finally:
            queue.put_nowait(None)

    producer = asyncio.create_task(produce())
    pending.add(producer)
    try:
        while (task := await queue.get()) is not None:
            # Await in submission order, even if later chunks finish first.
            for audio_chunk in await task:
                print(f"Yielding audio chunk of shape {audio_chunk.shape}")
                yield audio_chunk
            pending.remove(task)
            slots.release()
        await producer  # Propagate errors from the text stream.
    finally:
        for task in pending:
            task.cancel()
        await asyncio.gather(*pending, return_exceptions=True)
        # Running executor calls cannot be cancelled; they finish in the background.


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
