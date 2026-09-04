from seoah.config import setup_config
from seoah.llm.session import Session

async def _main() -> None:
    setup_config()

    ses = Session()

    for inp in [lambda: "안녕", input, input]:
        ses.add_user_input(inp())

        async for chunk in ses.next():
            # print(chunk)
            pass

        print(ses.conversations[-1])


def main() -> None:
    import asyncio

    asyncio.run(_main())