"""Lazy backend selection for the Discord text-to-OGG pipeline."""

from collections.abc import AsyncGenerator
from contextlib import aclosing

from seoah.config import load_config


async def convert_text_to_ogg(
    generator: AsyncGenerator[str],
) -> AsyncGenerator[bytes]:
    backend = load_config().tts_backend
    if backend in {"cosyvoice_cpp", "cosyvoice"}:
        if backend == "cosyvoice_cpp":
            from seoah.audio.cosyvoice_cpp import generate_cosyvoice_cpp_tts as generate
        else:
            from seoah.audio.cosyvoice import generate_cosyvoice_tts as generate

        async for text in generator:
            if text.strip():
                yield await generate(text)
        return

    if backend == "supertone":
        from seoah.audio.supertone import convert_to_audio, convert_to_ogg
    else:
        from seoah.audio.omnivoice import convert_to_audio, convert_to_ogg

    async with (
        aclosing(convert_to_audio(generator)) as audio,
        aclosing(convert_to_ogg(audio)) as ogg,
    ):
        async for chunk in ogg:
            yield chunk
