"""Async, CPU-only bridge to an isolated, persistent CosyVoice worker."""

import asyncio
import json
import os
from contextlib import suppress
from pathlib import Path
from tempfile import TemporaryDirectory

from seoah.config import ConfigFile, load_config


class CosyVoiceWorker:
    def __init__(self) -> None:
        self._process: asyncio.subprocess.Process | None = None
        self._lock = asyncio.Lock()

    async def _stop(self) -> None:
        process, self._process = self._process, None
        if process is None:
            return
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

    async def _reply(self) -> dict:
        assert self._process is not None and self._process.stdout is not None
        line = await self._process.stdout.readline()
        if not line:
            raise RuntimeError("CosyVoice worker exited; see stderr for details")
        try:
            reply = json.loads(line)
        except ValueError as error:
            raise RuntimeError("Invalid CosyVoice worker response") from error
        if not isinstance(reply, dict):
            raise TypeError("Invalid CosyVoice worker response")
        if "error" in reply:
            raise RuntimeError(f"CosyVoice synthesis failed: {reply['error']}")
        return reply

    async def _send(self, data: dict) -> None:
        assert self._process is not None and self._process.stdin is not None
        self._process.stdin.write((json.dumps(data) + "\n").encode())
        await self._process.stdin.drain()

    async def _start(self, config: ConfigFile) -> None:
        repo = Path(config.cosyvoice_repo).expanduser().resolve()

        def resolve(value: str) -> Path:
            path = Path(value).expanduser()
            # Do not resolve the Python symlink: it identifies the virtualenv.
            return path if path.is_absolute() else repo / path

        python = resolve(config.cosyvoice_python)
        model = resolve(config.cosyvoice_model)
        prompt = resolve(config.cosyvoice_prompt_wav)
        for path in (repo / "cosyvoice", python, model, prompt):
            if not path.exists():
                raise FileNotFoundError(
                    f"CosyVoice path not found: {path}; check cosyvoice_* configuration"
                )
        env = os.environ.copy()
        env.update(
            {
                "CUDA_VISIBLE_DEVICES": "",  # CPU even on CUDA-equipped machines.
                "OMP_NUM_THREADS": str(config.cosyvoice_threads),
                "PYTHONPATH": os.pathsep.join(
                    (str(repo), str(repo / "third_party/Matcha-TTS"))
                ),
                "PYTHONUNBUFFERED": "1",
            }
        )
        self._process = await asyncio.create_subprocess_exec(
            str(python),
            str(Path(__file__).with_name("_cosyvoice_worker.py")),
            cwd=repo,
            env=env,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            # Inherit stderr so model diagnostics cannot fill an unread pipe.
        )
        await self._send(
            {
                "model": str(model),
                "prompt_wav": str(prompt),
                "prompt_text": config.cosyvoice_prompt_text,
                "threads": config.cosyvoice_threads,
            }
        )
        if not (await self._reply()).get("ready"):
            raise RuntimeError("CosyVoice worker did not initialize")

    async def synthesize(self, text: str) -> bytes:
        if not text.strip():
            return b""
        config = load_config()
        async with self._lock:
            # Keep the output alive until cancellation has stopped the worker.
            with TemporaryDirectory(prefix="seoah-cosyvoice-") as directory:
                output = Path(directory) / "speech.ogg"
                try:
                    async with asyncio.timeout(config.cosyvoice_timeout):
                        if (
                            self._process is None
                            or self._process.returncode is not None
                        ):
                            await self._stop()
                            await self._start(config)
                        await self._send({"text": text, "output": str(output)})
                        if not (await self._reply()).get("ok"):
                            raise RuntimeError("Invalid CosyVoice synthesis response")
                        return await asyncio.to_thread(output.read_bytes)
                except BaseException:
                    # A cancelled/timed-out request must not leave a stale reply
                    # for the next caller, or keep expensive inference running.
                    await self._stop()
                    raise


_worker = CosyVoiceWorker()


async def generate_cosyvoice_tts(text: str) -> bytes:
    """Return a complete OGG/Vorbis clip at the model's native sample rate."""
    return await _worker.synthesize(text)


async def shutdown_cosyvoice() -> None:
    await _worker.close()
