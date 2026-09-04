from typing import Optional

from google.genai import Client

from seoah.config import load_config

client: Optional[Client] = None

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