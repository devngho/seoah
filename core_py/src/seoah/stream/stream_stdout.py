from typing import AsyncGenerator


async def stream_stdout(generator: AsyncGenerator[str, None]) -> None:
    """
    Asynchronously streams text from the provided generator to standard output.

    :param generator: An asynchronous generator that yields strings.
    """
    async for chunk in generator:
        print(chunk, end="", flush=True)
        # print(chunk) # for debugging


async def stream_stdout_passthrough(generator: AsyncGenerator[str, None]) -> AsyncGenerator[str, None]:
    """
    Asynchronously streams text from the provided generator to standard output and yields the same text.

    :param generator: An asynchronous generator that yields strings.
    :yield: The same text that is streamed to standard output.
    """
    async for chunk in generator:
        print(chunk, end="", flush=True)
        yield chunk