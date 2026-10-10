from collections.abc import AsyncGenerator
from typing import cast, Dict

from seoah.llm.session import Interaction, TextSessionPart

async def chunk_by(generator: AsyncGenerator[str, None], to_split: list[str]):
    """
    Asynchronously yields chunks of text from the provided generator, splitting the text based on the specified delimiters.

    :param generator: An asynchronous generator that yields strings.
    :param to_split: A list of strings to split the text by.
    :yield: Chunks of text split by the specified delimiters.
    """

    if any(not delimiter for delimiter in to_split):
        raise ValueError("Chunk delimiters must not be empty")

    buf = ""

    async for chunk in generator:
        buf += chunk
        while True:
            matches = [
                (buf.find(delimiter), delimiter)
                for delimiter in to_split
            ]
            matches = [
                (index, delimiter)
                for index, delimiter in matches
                if index >= 0
            ]
            if not matches:
                break
            index, delimiter = min(matches, key=lambda match: match[0])
            end = index + len(delimiter)
            yield buf[:end]
            buf = buf[end:]

    if buf:
        yield buf


async def strip(generator: AsyncGenerator[str, None]):
    """
    Asynchronously yields text from the provided generator, stripping leading and trailing whitespace.

    :param generator: An asynchronous generator that yields strings.
    :yield: Text with leading and trailing whitespace removed.
    """

    async for chunk in generator:
        text = chunk.strip()
        if len(text) > 0:
            yield text


async def extract_output_from_interactions(
        generator: AsyncGenerator[Interaction, None],
) -> AsyncGenerator[str, None]:
    """
    Asynchronously extracts and concatenates text from a stream of SessionPart objects.

    :param generator: An asynchronous generator that yields SessionPart objects.
    :return: A concatenated string of all text parts extracted from the SessionPart objects.
    """

    last_len = 0
    async for interaction in generator:
        if len(interaction.contents) > 0:
            # if last part is TextSessionPart, concatenate the text to the last part
            if isinstance(interaction.contents[-1], TextSessionPart):
                text_part = cast(
                    TextSessionPart, interaction.contents[-1]
                )
                if len(text_part.text) > last_len:
                    yield text_part.text[last_len:]
                    last_len = len(text_part.text)
            else:
                last_len = 0


async def replace(generator: AsyncGenerator[str, None], mapping: Dict[str, str]) -> AsyncGenerator[str, None]:
    """
    Asynchronously yields text from the provided generator, replacing specified substrings based on a mapping.

    :param generator: An asynchronous generator that yields strings.
    :param mapping: A dictionary where keys are substrings to be replaced and values are their replacements.
    :yield: Text with specified substrings replaced according to the mapping.
    """

    async for chunk in generator:
        for old, new in mapping.items():
            chunk = chunk.replace(old, new)

        yield chunk


async def join(generator: AsyncGenerator[str, None]) -> AsyncGenerator[str, None]:
    """
    Asynchronously joins text from the provided generator into a single string.

    :param generator: An asynchronous generator that yields strings.
    :yield: A single concatenated string of all text parts.
    """

    buf = ""
    async for chunk in generator:
        buf += chunk

    yield buf