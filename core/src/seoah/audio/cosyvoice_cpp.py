"""Persistent cosyvoice.cpp HTTP provider with optional owned local server."""

import asyncio
import secrets
import socket
from contextlib import suppress
from io import BytesIO
from pathlib import Path

import httpx
import numpy as np
import soundfile as sf

from seoah.config import ConfigFile, load_config

MAX_AUDIO_BYTES = 32 * 1024 * 1024
MAX_AUDIO_SAMPLES = 24_000 * 600


def wav_to_ogg(wav: bytes) -> bytes:
    """Validate bounded WAV audio and preserve its native sample rate."""
    with sf.SoundFile(BytesIO(wav)) as source:
        if (
            source.format not in {"WAV", "WAVEX", "RF64"}
            or source.channels not in {1, 2}
            or not 8000 <= source.samplerate <= 192000
            or not 0 < source.frames * source.channels <= MAX_AUDIO_SAMPLES
        ):
            raise ValueError("Invalid or oversized cosyvoice.cpp WAV response")
        rate = source.samplerate
        audio = source.read(dtype="float32", always_2d=True)
    if not np.isfinite(audio).all():
        raise ValueError("cosyvoice.cpp returned non-finite audio")
    with BytesIO() as output:
        sf.write(output, audio, rate, format="OGG", subtype="VORBIS")
        return output.getvalue()


class CosyVoiceCppProvider:
    def __init__(self) -> None:
        self._lock = asyncio.Lock()
        self._process: asyncio.subprocess.Process | None = None
        self._client: httpx.AsyncClient | None = None

    async def _stop(self) -> None:
        client, self._client = self._client, None
        process, self._process = self._process, None
        try:
            if client is not None:
                await client.aclose()
        finally:
            if process is not None:
                if process.returncode is None:
                    with suppress(ProcessLookupError):
                        process.terminate()
                try:
                    await asyncio.wait_for(process.wait(), timeout=5)
                except TimeoutError:
                    with suppress(ProcessLookupError):
                        process.kill()
                    await process.wait()

    async def close(self) -> None:
        async with self._lock:
            await self._stop()

    def _command(self, config: ConfigFile, port: int, token: str) -> list[str]:
        repo = Path(config.cosyvoice_cpp_repo).expanduser().resolve()

        def resolve(value: str, directory: bool = False) -> str:
            path = Path(value).expanduser()
            path = path if path.is_absolute() else repo / path
            if not (path.is_dir() if directory else path.is_file()):
                raise FileNotFoundError(f"CosyVoice.cpp path not found: {path}")
            return str(path)

        return [
            resolve(config.cosyvoice_cpp_binary),
            "--api",
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
            "--api-key",
            token,
            "--model",
            resolve(config.cosyvoice_cpp_model),
            "--voice-prompt",
            f"{config.cosyvoice_cpp_voice}={resolve(config.cosyvoice_cpp_prompt)}",
            "--served-model-name",
            config.cosyvoice_cpp_model_name,
            "--backend",
            config.cosyvoice_cpp_backend,
            "--backend-path",
            resolve(config.cosyvoice_cpp_backend_path, directory=True),
            "--threads",
            str(config.cosyvoice_cpp_threads),
            "--llm-kv-cache-type",
            "f16",
            "--inference-buffer-policy",
            "dedicated",
        ]

    async def _wait_ready(self, config: ConfigFile) -> None:
        if self._process is None or self._client is None:
            raise RuntimeError("CosyVoice.cpp managed server was not started")
        async with asyncio.timeout(config.cosyvoice_cpp_startup_timeout):
            while True:
                if self._process.returncode is not None:
                    raise RuntimeError(
                        "CosyVoice.cpp server exited; see its stderr logs"
                    )
                try:
                    response = await self._client.get("v1/models", timeout=1)
                    response.raise_for_status()
                    return
                except httpx.TransportError:
                    await asyncio.sleep(0.1)

    async def _start(self, config: ConfigFile) -> None:
        if config.cosyvoice_cpp_url is not None:
            url = str(config.cosyvoice_cpp_url).rstrip("/") + "/"
            token = (
                config.cosyvoice_cpp_api_key.get_secret_value()
                if config.cosyvoice_cpp_api_key
                else None
            )
        else:
            # The server cannot inherit a listener. A random auth token also
            # prevents mistaking another service for ours if the port is raced.
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            token = secrets.token_urlsafe(32)
            command = self._command(config, port, token)
            self._process = await asyncio.create_subprocess_exec(
                *command,
                cwd=Path(config.cosyvoice_cpp_repo).expanduser(),
                stdin=asyncio.subprocess.DEVNULL,
                # Inherit logs; never leave an unread subprocess pipe.
            )
            url = f"http://127.0.0.1:{port}/"
        self._client = httpx.AsyncClient(
            base_url=url,
            headers={"Authorization": f"Bearer {token}"} if token else {},
            timeout=config.cosyvoice_cpp_timeout,
            trust_env=False,
        )
        if self._process is not None:
            await self._wait_ready(config)

    async def _request(self, config: ConfigFile, text: str) -> bytes:
        if self._client is None:
            raise RuntimeError("CosyVoice.cpp HTTP client was not started")
        async with self._client.stream(
            "POST",
            "v1/audio/speech",
            json={
                "model": config.cosyvoice_cpp_model_name,
                "voice": config.cosyvoice_cpp_voice,
                "input": text,
                "response_format": "wav",
                "stream": False,
            },
        ) as response:
            response.raise_for_status()
            audio = bytearray()
            async for chunk in response.aiter_bytes():
                if len(audio) + len(chunk) > MAX_AUDIO_BYTES:
                    raise ValueError("CosyVoice.cpp response exceeds audio size limit")
                audio.extend(chunk)
        return await asyncio.to_thread(wav_to_ogg, bytes(audio))

    async def synthesize(self, text: str) -> bytes:
        if not text.strip():
            return b""
        config = load_config()
        async with self._lock:
            try:
                if self._process is not None and self._process.returncode is not None:
                    await self._stop()
                if self._client is None:
                    await self._start(config)
                async with asyncio.timeout(config.cosyvoice_cpp_timeout):
                    return await self._request(config, text)
            except BaseException:
                # Disconnect on cancellation and terminate only servers we own.
                # No stale inference can overlap the next owned-server request.
                await self._stop()
                raise


_provider = CosyVoiceCppProvider()


async def generate_cosyvoice_cpp_tts(text: str) -> bytes:
    return await _provider.synthesize(text)


async def shutdown_cosyvoice_cpp() -> None:
    await _provider.close()
