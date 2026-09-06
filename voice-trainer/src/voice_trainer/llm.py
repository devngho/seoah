from typing import Optional

from google.genai import Client, types

from voice_trainer.config import load_config

import json

client: Optional[Client] = None


def get_client() -> Client:
    """
    Get the global client instance. If it doesn't exist, create it using the API key from the configuration.

    :return: The global Client instance.
    """
    global client

    c: Client

    config = load_config()

    if client is None or client._api_client.api_key != config.api_key:  # hot reload the client if the API key has changed
        c = Client(api_key=config.api_key)
        client = c
    else:
        c = client

    return c


async def generate_sentences() -> list[str]:
    """
    Generate a list of sentences based on the provided prompt.

    :return: A list of generated sentences.
    """
    config = load_config()
    client = get_client()

    response = await client.aio.models.generate_content(
        model=config.backend_model,
        contents=config.instruction_for_generation.format(count=config.num_sentence_to_generate),
        config=types.GenerateContentConfig(
            thinking_config=types.ThinkingConfig(
                thinking_level=config.thinking_effort
            ),
            response_mime_type="application/json",
            response_schema=types.Schema(
                type=types.Type.OBJECT,
                properties={
                    "sentences": types.Schema(
                        type=types.Type.ARRAY,
                        items=types.Schema(
                            type=types.Type.STRING
                        ),
                        min_length=config.num_sentence_to_generate,
                        max_length=config.num_sentence_to_generate
                    )
                },
                required=["sentences"]
            ),
            system_instruction=config.prompt
        )
    )

    return response.parsed.get("sentences", [])