import asyncio
from asyncio import Task

import discord

from seoah.config import load_config
from seoah.llm.session import Session
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

@client.event
async def on_message(message):
    config = load_config()

    if message.author == client.user:
        return

    if message.content == "[seoah off]":
        toggle_by_channel[message.channel.id] = False
        return

    if message.content == "[seoah on]":
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
            stream = extract_output_from_interactions(stream)

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
