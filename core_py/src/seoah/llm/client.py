from dataclasses import dataclass, fields, replace
from typing import AsyncGenerator, Optional

from google.genai import Client, types

from seoah.config import load_config

client: Optional[Client] = None


@dataclass(frozen=True)
class TokenMetrics:
    """Provider-reported token totals; cached tokens are a subset of input tokens."""

    prompt_token_count: int = 0
    candidates_token_count: int = 0
    thoughts_token_count: int = 0
    cached_content_token_count: int = 0
    tool_use_prompt_token_count: int = 0
    total_token_count: int = 0


_token_metrics = TokenMetrics()


def get_token_metrics() -> TokenMetrics:
    """Return an immutable process-wide snapshot, including usage from active streams."""
    return _token_metrics


async def generate_content_stream(
    *,
    model: str,
    contents: types.ContentListUnion,
    config: types.GenerateContentConfigOrDict | None = None,
) -> AsyncGenerator[types.GenerateContentResponse, None]:
    """Stream responses unchanged and accumulate reported usage at the client level.

    Streaming usage is cumulative per request, not additive per chunk. Missing
    metadata is not estimated; interrupted streams retain only reported usage.
    """
    global _token_metrics

    stream = await get_client().aio.models.generate_content_stream(
        model=model, contents=contents, config=config,
    )
    previous = TokenMetrics()
    try:
        async for chunk in stream:
            if chunk.usage_metadata is not None:
                reported = {
                    field.name: value
                    for field in fields(TokenMetrics)
                    if (value := getattr(chunk.usage_metadata, field.name, None)) is not None
                }
                current = replace(previous, **reported)
                _token_metrics = TokenMetrics(**{
                    field.name: getattr(_token_metrics, field.name)
                    + getattr(current, field.name) - getattr(previous, field.name)
                    for field in fields(TokenMetrics)
                })
                previous = current
            yield chunk
    finally:
        await stream.aclose()

def get_client() -> Client:
    """
    Get the global client instance. If it doesn't exist, create it using the API key from the configuration.

    :return: The global Client instance.
    """
    global client

    c: Client

    config = load_config()
    
    if client is None or client._api_client.api_key != config.api_key: # hot reload the client if the API key has changed
        c = Client(api_key=config.api_key)
        client = c
    else:
        c = client

    return c