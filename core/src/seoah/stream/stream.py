from typing import AsyncGenerator, List, cast

from seoah.llm.session import Interaction, TextSessionPart


async def replace(generator: AsyncGenerator[str, None], to_remove: list[str], to_replace: str):
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


async def chunk_by(generator: AsyncGenerator[str, None], to_split: List[str]):
    """
    Asynchronously yields chunks of text from the provided generator, splitting the text based on the specified delimiters.

    :param generator: An asynchronous generator that yields strings.
    :param to_split: A list of strings to split the text by.
    :yield: Chunks of text split by the specified delimiters.
    """

    buf = ""

    async for chunk in generator:
        buf += chunk
        while True:
            min_index = len(buf)
            for split_str in to_split:
                index = buf.find(split_str)
                if index != -1 and index < min_index:
                    min_index = index + len(split_str)
            if min_index < len(buf):
                yield buf[:min_index]
                buf = buf[min_index:]
            else:
                break

    if buf:
        yield buf


async def extract_output_from_interactions(generator: AsyncGenerator[Interaction, None]) -> AsyncGenerator[str, None]:
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
                text_part = cast(TextSessionPart, interaction.contents[-1])
                if len(text_part.text) > last_len:
                    yield text_part.text[last_len:]
                    last_len = len(text_part.text)
            else:
                last_len = 0