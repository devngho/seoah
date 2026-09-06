import asyncio
import signal
from contextlib import suppress

from seoah.audio.cosyvoice import shutdown_cosyvoice
from seoah.audio.cosyvoice_cpp import shutdown_cosyvoice_cpp
from seoah.config import setup_config
from seoah.discord.bot import setup_discord_bot, shutdown_discord_bot
from seoah.log import log


async def _main(config_path: str | None = None) -> None:
    loop = asyncio.get_running_loop()

    setup_config(config_path)

    main_task = asyncio.current_task()
    assert main_task is not None

    def request_shutdown():
        # Repeated signals must not interrupt cleanup.
        if not main_task.cancelling():
            main_task.cancel()

    installed_signals = []
    for sig in (signal.SIGINT, signal.SIGTERM):
        try:
            loop.add_signal_handler(sig, request_shutdown)
            installed_signals.append(sig)
        except NotImplementedError:
            pass

    try:
        await setup_discord_bot()
    finally:
        log(lambda: "Shutting down...", "INFO")
        try:
            await shutdown_discord_bot()
        finally:
            try:
                try:
                    await shutdown_cosyvoice_cpp()
                finally:
                    await shutdown_cosyvoice()
            finally:
                for sig in installed_signals:
                    loop.remove_signal_handler(sig)


def main(config_path: str | None = None) -> None:
    with suppress(KeyboardInterrupt, asyncio.CancelledError):
        asyncio.run(_main(config_path))
