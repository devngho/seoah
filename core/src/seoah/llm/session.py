from asyncio import Lock
from dataclasses import dataclass
from typing import Any, AsyncGenerator, override

from google.genai.types import ContentUnionDict, Content, Part

from seoah.config import ConfigFile, load_config
from seoah.llm.client import get_client
from seoah.log import log


class SessionPart:
    def prepare_part_gemini(self) -> Part:
        """
        Prepare the part for sending to the language model.
        :return: A ContentUnionDict representing the part.
        """

        raise NotImplementedError("Subclasses must implement this method.")


@dataclass()
class TextSessionPart(SessionPart):
    """
    A part of the response from the language model that contains text.
    """
    text: str

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(text=self.text)


@dataclass()
class ThoughtSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a thought.
    """
    thought: str

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(text=self.thought, thought=True)


@dataclass()
class RawSessionPart(SessionPart):
    """
    A part of the response from the language model that contains provider-specific raw data, such as a thought signature.
    """
    raw: Any

    @override
    def prepare_part_gemini(self) -> Part:
        return self.raw

    def __str__(self) -> str:
        return "RawSessionPart"


@dataclass()
class ToolCallSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a tool call.
    """
    tool_name: str
    tool_args: dict[str, Any]


@dataclass()
class ToolResultSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a tool result.
    """
    tool_name: str
    tool_result: Any


@dataclass()
class Interaction:
    """
    An interaction is a user input or output from the language model.
    """
    role: str
    contents: list[SessionPart]
    is_in_progress: bool = False

    def __str__(self) -> str:
        content_strs = []
        for content in self.contents:
            if isinstance(content, TextSessionPart):
                content_strs.append(f"text | {content.text}")
            elif isinstance(content, ThoughtSessionPart):
                content_strs.append(f"thought | {content.thought}")
            elif isinstance(content, RawSessionPart):
                content_strs.append("raw")
            elif isinstance(content, ToolCallSessionPart):
                content_strs.append(f"Tool Call: {content.tool_name} with args {content.tool_args}")
            elif isinstance(content, ToolResultSessionPart):
                content_strs.append(f"Tool Result: {content.tool_name} with result {content.tool_result}")
            else:
                content_strs.append("Unknown Content Type")
        return f"Interaction(role={self.role}, is_in_progress={self.is_in_progress}, contents=[\n{'\n'.join(content_strs)}\n])"

class Session:
    """
    A session contains all the information needed to run a conversation with a language model. Sessions can be streamed, interrupted, and resumed.
    """

    conversations: list[Interaction] = []
    _mutex = Lock()

    def __str__(self) -> str:
        return f"Session(conversations=[\n{'\n'.join(str(c) for c in self.conversations)}\n])"

    def prepare_contents_gemini(self) -> list[ContentUnionDict]:
        """
        Prepare the contents for the Gemini model based on the current conversation history.
        :return: A list of contents to be sent to the model.
        """
        contents: list[ContentUnionDict] = []
        for interaction in self.conversations:
            contents.append(Content(
                role=interaction.role,
                parts=list(map(lambda p: p.prepare_part_gemini(), interaction.contents))
            ))

        return contents

    def add_user_input(self, user_input: str) -> None:
        """
        Add user input to the conversation history.
        :param user_input: The input from the user.
        :return: None
        """
        if self.conversations and self.conversations[-1].role == "user":
            # If the last interaction is a user input, append to it
            if len(self.conversations[-1].contents) > 0 and isinstance(self.conversations[-1].contents[-1], TextSessionPart):
                self.conversations[-1].contents[-1].text += "\n" + user_input
            else:
                self.conversations[-1].contents.append(TextSessionPart(text=user_input))

            return

        interaction = Interaction(
            role="user",
            contents=[TextSessionPart(text=user_input)],
            is_in_progress=False
        )

        self.conversations.append(interaction)

    async def next(self) -> AsyncGenerator[Interaction, Any]:
        """
        Continue the conversation with the language model and return the next response stream.
        :return:
        """
        config = load_config()

        if self._mutex.locked():
            raise RuntimeError("Session is already running. Please wait for the current session to finish before starting a new one.")
        else:
            async with self._mutex:
                from google.genai import types
                response = await get_client().aio.models.generate_content_stream(
                    model=config.backend_model,
                    contents=self.prepare_contents_gemini(),
                    config=types.GenerateContentConfig(
                        system_instruction=config.prompt,
                        thinking_config=types.ThinkingConfig(include_thoughts=True, thinking_level=config.thinking_effort)
                    ),
                )

                # create a new interaction for the response
                interaction = Interaction(
                    role="model",
                    contents=[],
                    is_in_progress=True
                )

                self.conversations.append(interaction)

                try:
                    async for chunk in response:
                        print(f"chunk: {chunk}")
                        if chunk.usage_metadata is not None:
                            log(lambda: f"Usage metadata: {chunk.usage_metadata}", "DEBUG")

                        res = chunk.candidates[0].content

                        if res is not None:
                            if res.parts is None:
                                continue

                            for p in res.parts:
                                if p.text is not None and p.text != "" and (p.thought is None or not p.thought):
                                    # append or update the text part in the interaction
                                    if len(interaction.contents) > 0 and isinstance(interaction.contents[-1], TextSessionPart):
                                        interaction.contents[-1].text += p.text
                                    else:
                                        interaction.contents.append(TextSessionPart(text=p.text))
                                elif p.thought:
                                    # append or update the thought part in the interaction
                                    if len(interaction.contents) > 0 and isinstance(interaction.contents[-1], ThoughtSessionPart):
                                        interaction.contents[-1].thought += p.text
                                    else:
                                        interaction.contents.append(ThoughtSessionPart(thought=p.text))
                                elif p.thought_signature is not None:
                                    interaction.contents.append(RawSessionPart(raw=p))
                                elif p.tool_call is not None:
                                    interaction.contents.append(ToolCallSessionPart(tool_name=p.tool_call.tool_name, tool_args=p.tool_call.tool_args))
                                elif p.tool_result is not None:
                                    interaction.contents.append(ToolResultSessionPart(tool_name=p.tool_result.tool_name, tool_result=p.tool_result.tool_result))

                            yield interaction
                except Exception as e:
                    yield Interaction(
                        role="model",
                        contents=[TextSessionPart(text=f"Error: {str(e)}")],
                        is_in_progress=False
                    )

                interaction.is_in_progress = False