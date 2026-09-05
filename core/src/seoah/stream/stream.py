from collections.abc import AsyncGenerator
from typing import cast

from seoah.llm.session import Interaction, TextSessionPart


async def replace(
        generator: AsyncGenerator[str, None], to_remove: list[str], to_replace: str
):
    """
    Asynchronously yields text from the provided generator, replacing specified substrings with a new string.

    :param generator: An asynchronous generator that yields strings.
    :param to_remove: A list of strings to replace.
    :param to_replace: The string to replace the specified substrings with.
    :yield: Text with specified substrings removed.
    """

    async for chunk in generator:
        for remove_str in to_remove:
            chunk = chunk.replace(remove_str, to_replace)
        yield chunk


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
