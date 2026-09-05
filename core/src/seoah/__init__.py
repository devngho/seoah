from typing import Optional

from seoah.audio.tts import load_model
from seoah.config import setup_config
from seoah.discord.bot import setup_discord_bot
from seoah.llm.session import Session
from seoah.stream.stream import chunk_by, extract_output_from_interactions
from seoah.stream.stream_stdout import stream_stdout


async def _main(config_path: Optional[str] = None) -> None:
    setup_config(config_path)

    ses = Session()

    await load_model()

    await setup_discord_bot()

    # for inp in [lambda: "안녕", lambda: input("> "), lambda: input("> ")]:
    #     ses.add_user_input(inp())
    #
    #     stream = ses.next()
    #     stream = extract_output_from_interactions(stream)
    #     stream = remove(stream, ["\n", "\r"])
    #     stream = chunk_by(stream, [".", "!", "?"])
    #     await stream_stdout(stream)
    #
    #     print("\n")
    #
    #     # print(ses.conversations[-1])


def main(config_path: Optional[str] = None) -> None:
    import asyncio

    asyncio.run(_main(config_path))