import asyncio
from asyncio import Task
from typing import AsyncGenerator, cast

import discord

from seoah.config import load_config
from seoah.llm.session import Session, Interaction, TextSessionPart, FunctionCallSessionPart
from seoah.log import log
from seoah.stream.stream import extract_output_from_interactions, chunk_by
from seoah.stream.stream_stdout import stream_stdout, stream_stdout_passthrough

intents = discord.Intents.default()
intents.message_content = True

client: discord.Client = discord.Client(intents=intents)
ses = Session()

debounce_task_by_channel: dict[int, Task] = {}
toggle_by_channel: dict[int, bool] = {}

@client.event
async def on_ready():
    log(lambda: f'Logged in as {client.user}', "INFO")


async def extract_output_from_interactions_discord(generator: AsyncGenerator[Interaction, None]) -> AsyncGenerator[str, None]:
    """
    Asynchronously extracts and concatenates text from a stream of SessionPart objects.

    :param generator: An asynchronous generator that yields SessionPart objects.
    :return: A concatenated string of all text parts extracted from the SessionPart objects.
    """

    last_len = 0
    async for interaction in generator:
        print(f"Received interaction: {interaction}")
        if len(interaction.contents) > 0:
            # if last part is TextSessionPart, concatenate the text to the last part
            if isinstance(interaction.contents[-1], TextSessionPart):
                text_part = cast(TextSessionPart, interaction.contents[-1])
                if len(text_part.text) > last_len:
                    yield text_part.text[last_len:]
                    last_len = len(text_part.text)
            elif isinstance(interaction.contents[-1], FunctionCallSessionPart):
                func_part = cast(FunctionCallSessionPart, interaction.contents[-1])
                yield f"`함수를 호출함: {func_part.tool_name} {func_part.tool_args}`\n"
            else:
                last_len = 0

@client.event
async def on_message(message):
    config = load_config()

    if message.author == client.user:
        return

    if message.content == "[seoah off]":
        log(lambda: f"Disabling seoah in channel {message.channel.name} ({message.channel.id})", "INFO")
        toggle_by_channel[message.channel.id] = False
        return

    if message.content == "[seoah on]":
        log(lambda: f"Enabling seoah in channel {message.channel.name} ({message.channel.id})", "INFO")
        toggle_by_channel[message.channel.id] = True
        return

    if not toggle_by_channel.get(message.channel.id, True):
        return

    async def task():
        await asyncio.sleep(config.debounce_ms / 1000)

        # show typing indicator
        async with message.channel.typing():
            print(f'current session: {ses}')
            stream = ses.next()
            stream = extract_output_from_interactions_discord(stream)

            stream = chunk_by(stream, ["\n", "\r", "!", "?"])
            stream = stream_stdout_passthrough(stream)

            async for chunk in stream:
                if chunk.strip() == "":
                    continue
                print(f"Sending chunk to Discord: {chunk}")

                await message.channel.send(chunk)
                await asyncio.sleep(0.5)  # slight delay

    if message.channel.id in debounce_task_by_channel:
        debounce_task_by_channel[message.channel.id].cancel()

        # cleanup last conversation, if the last interaction is in progress but empty
        if ses.conversations and ses.conversations[-1].is_in_progress and not ses.conversations[-1].contents:
            ses.conversations.pop()

    ses.add_user_input(f"{message.author} said: {message.content}")
    debounce_task_by_channel[message.channel.id] = asyncio.create_task(task())

async def setup_discord_bot():
    from seoah.config import load_config
    config = load_config()

    if not config.discord_api_key:
        log(lambda: "Discord API key is not set in the configuration. Discord bot will not be started.", "WARNING")
        return

    await client.start(config.discord_api_key)
