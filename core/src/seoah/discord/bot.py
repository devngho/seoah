import asyncio
import traceback
from asyncio import Task
from collections.abc import AsyncGenerator
from contextlib import aclosing, closing
from io import BytesIO

import discord

from seoah.audio.tts import convert_text_to_ogg
from seoah.config import load_config
from seoah.llm.session import (
    FunctionCallSessionPart,
    Interaction,
    Session,
    TextSessionPart,
)
from seoah.log import log
from seoah.stream.stream import chunk_by, strip
from seoah.stream.stream_stdout import stream_stdout_passthrough

intents = discord.Intents.default()
intents.message_content = True

client: discord.Client = discord.Client(intents=intents)
ses = Session()

debounce_task_by_channel: dict[int, Task] = {}
toggle_by_channel: dict[int, bool] = {}
response_tasks: set[Task] = set()
_shutting_down = False


@client.event
async def on_ready():
    config = load_config()

    log(lambda: f"Logged in as {client.user}", "INFO")
    await client.change_presence(
        status=discord.Status.online, activity=discord.Game(name=config.backend_model)
    )


async def extract_output_from_interactions_discord(
    generator: AsyncGenerator[Interaction, None],
) -> tuple[AsyncGenerator[str, None], AsyncGenerator[str, None]]:
    """Return text and function-call streams.

    The text stream drives the source; consume metadata alongside it or afterward.
    """
    metadata: asyncio.Queue[str | None] = asyncio.Queue()

    async def text_stream() -> AsyncGenerator[str, None]:
        previous: Interaction | None = None
        position, last_len = 0, 0

        try:
            async for interaction in generator:
                if interaction is not previous:
                    previous = interaction
                    position, last_len = 0, 0

                while position < len(interaction.contents):
                    part = interaction.contents[position]

                    if isinstance(part, TextSessionPart):
                        delta = part.text[last_len:]
                        last_len = len(part.text)
                        if delta:
                            yield delta
                        # The final text part may grow on the next update.
                        if position == len(interaction.contents) - 1:
                            break
                    elif isinstance(part, FunctionCallSessionPart):
                        metadata.put_nowait(
                            f"`함수를 호출함: {part.tool_name} {part.tool_args}`\n"
                        )

                    position += 1
                    last_len = 0
        finally:
            metadata.put_nowait(None)

    async def meta_stream() -> AsyncGenerator[str, None]:
        while (message := await metadata.get()) is not None:
            yield message

    return text_stream(), meta_stream()


@client.event
async def on_message(message):
    if _shutting_down:
        return

    config = load_config()

    if message.author == client.user:
        return

    if message.content == "[seoah off]":
        log(
            lambda: (
                f"Disabling seoah in channel {message.channel.name} ({message.channel.id})"
            ),
            "INFO",
        )
        toggle_by_channel[message.channel.id] = False
        return

    if message.content == "[seoah on]":
        log(
            lambda: (
                f"Enabling seoah in channel {message.channel.name} ({message.channel.id})"
            ),
            "INFO",
        )
        toggle_by_channel[message.channel.id] = True
        return

    if not toggle_by_channel.get(message.channel.id, True):
        return

    async def task():
        await asyncio.sleep(config.debounce_ms / 1000)

        # show typing indicator
        async with message.channel.typing():
            print(f"current session: {ses}")
            stream = ses.next()
            text_stream, meta_stream = await extract_output_from_interactions_discord(
                stream
            )

            chunks = chunk_by(text_stream, [".", "\n", "\r", "!", "?"])
            striped_chunks = strip(chunks)

            ogg_stream = convert_text_to_ogg(striped_chunks)

            async def send_text(output: AsyncGenerator[str, None]):
                async for chunk in stream_stdout_passthrough(output):
                    if chunk.strip() == "":
                        continue
                    print(f"Sending chunk to Discord: {chunk}")
                    await message.channel.send(chunk)
                    await asyncio.sleep(0.5)  # slight delay

            async def send_audio(output: AsyncGenerator[bytes, None]):
                async for chunk in output:
                    if not chunk:
                        continue
                    print(f"Sending audio chunk to Discord: {len(chunk)} bytes")
                    with (
                        BytesIO(chunk) as buffer,
                        closing(discord.File(buffer, filename="output.ogg")) as file,
                    ):
                        await message.channel.send(file=file)

                    await asyncio.sleep(0.5)  # slight delay

            async with (
                aclosing(stream),
                aclosing(meta_stream),
                aclosing(text_stream),
                aclosing(chunks),
                aclosing(striped_chunks),
                aclosing(ogg_stream),
                asyncio.TaskGroup() as group,
            ):
                # group.create_task(send_text(text_stream))
                group.create_task(send_audio(ogg_stream))
                group.create_task(send_text(meta_stream))

    if message.channel.id in debounce_task_by_channel:
        debounce_task_by_channel[message.channel.id].cancel()

        # cleanup last conversation, if the last interaction is in progress but empty
        if (
            ses.conversations
            and ses.conversations[-1].is_in_progress
            and not ses.conversations[-1].contents
        ):
            ses.conversations.pop()

    ses.add_user_input(f"{message.author} said: {message.content}")

    def report_failure(completed: Task):
        response_tasks.discard(completed)
        if debounce_task_by_channel.get(message.channel.id) is completed:
            debounce_task_by_channel.pop(message.channel.id)
        if not completed.cancelled() and (error := completed.exception()) is not None:
            traceback.print_exception(error)

    response_task = asyncio.create_task(task())
    response_tasks.add(response_task)
    response_task.add_done_callback(report_failure)
    debounce_task_by_channel[message.channel.id] = response_task


async def setup_discord_bot():
    from seoah.config import load_config

    config = load_config()

    if not config.discord_api_key:
        log(
            lambda: (
                "Discord API key is not set in the configuration. Discord bot will not be started."
            ),
            "WARNING",
        )
        return

    await client.start(config.discord_api_key)


async def shutdown_discord_bot():
    global _shutting_down
    _shutting_down = True
    tasks = tuple(response_tasks)
    for task in tasks:
        if not task.cancelling():
            task.cancel()
    try:
        await asyncio.gather(*tasks, return_exceptions=True)
    finally:
        response_tasks.clear()
        debounce_task_by_channel.clear()

        await client.close()
