from asyncio import Lock
from dataclasses import dataclass
from symtable import Function
from typing import Any, AsyncGenerator, override

from google.genai.types import ContentUnionDict, Content, Part
from google.genai import types

from seoah.config import ConfigFile, load_config
from seoah.llm.client import get_client
from seoah.log import log


class SessionPart:
    extra_fields: dict[str, Any]

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
    extra_fields: dict[str, Any]

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(text=self.text, **self.extra_fields)


@dataclass()
class ThoughtSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a thought.
    """
    thought: str
    extra_fields: dict[str, Any]

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(text=self.thought, thought=True, **self.extra_fields)


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
class FunctionCallSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a tool call.
    """
    tool_name: str
    tool_args: dict[str, Any]
    extra_fields: dict[str, Any]

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(function_call=types.FunctionCall(name=self.tool_name, args=self.tool_args), **self.extra_fields)


@dataclass()
class FunctionResultSessionPart(SessionPart):
    """
    A part of the response from the language model that contains a tool result.
    """
    tool_name: str
    tool_result: Any
    extra_fields: dict[str, Any]

    @override
    def prepare_part_gemini(self) -> Part:
        return Part(function_response=types.FunctionResponse(name=self.tool_name, response=self.tool_result),
                    **self.extra_fields)


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
            elif isinstance(content, FunctionCallSessionPart):
                content_strs.append(f"Tool Call: {content.tool_name} with args {content.tool_args}")
            elif isinstance(content, FunctionResultSessionPart):
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
            if len(self.conversations[-1].contents) > 0 and isinstance(self.conversations[-1].contents[-1],
                                                                       TextSessionPart):
                self.conversations[-1].contents[-1].text += "\n" + user_input
            else:
                self.conversations[-1].contents.append(TextSessionPart(text=user_input, extra_fields={}))

            return

        interaction = Interaction(
            role="user",
            contents=[TextSessionPart(text=user_input, extra_fields={})],
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
            raise RuntimeError(
                "Session is already running. Please wait for the current session to finish before starting a new one.")
        else:
            async with self._mutex:
                keep_working = True

                while keep_working:
                    keep_working = False
                    print(f"Preparing contents for Gemini model: {self.prepare_contents_gemini()}")

                    response = await get_client().aio.models.generate_content_stream(
                        model=config.backend_model,
                        contents=self.prepare_contents_gemini(),
                        config=types.GenerateContentConfig(
                            tools=[
                                types.Tool(url_context=types.UrlContext(

                                )),
                                types.Tool(function_declarations=[
                                    types.FunctionDeclaration(
                                        name="start_gomoku_game",
                                        description="Start a new game of Gomoku. You must call this function to play the game.",
                                    )
                                ])
                            ],
                            tool_config=types.ToolConfig(
                                include_server_side_tool_invocations=True
                            ),
                            system_instruction=config.prompt,
                            thinking_config=types.ThinkingConfig(include_thoughts=True,
                                                                 thinking_level=config.thinking_effort)
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
                                    extra = {"thought_signature": p.thought_signature} if p.thought_signature else {}

                                    if p.text is not None and (p.thought is None or not p.thought):
                                        # append or update the text part in the interaction
                                        if len(interaction.contents) > 0 and isinstance(interaction.contents[-1],
                                                                                        TextSessionPart):
                                            interaction.contents[-1].text += p.text
                                        else:
                                            interaction.contents.append(
                                                TextSessionPart(text=p.text, extra_fields=extra))
                                    elif p.thought:
                                        # append or update the thought part in the interaction
                                        if len(interaction.contents) > 0 and isinstance(interaction.contents[-1],
                                                                                        ThoughtSessionPart):
                                            interaction.contents[-1].thought += p.text
                                        else:
                                            interaction.contents.append(
                                                ThoughtSessionPart(thought=p.text, extra_fields=extra))
                                    elif p.function_call is not None:
                                        log(lambda: f"Function call detected: {p.function_call.name} with args {p.function_call.args}",
                                            "INFO")
                                        interaction.contents.append(
                                            FunctionCallSessionPart(tool_name=p.function_call.name,
                                                                    tool_args=p.function_call.args, extra_fields=extra))

                                        yield interaction

                                        interaction = Interaction(
                                            role="user",
                                            contents=[
                                                FunctionResultSessionPart(tool_name=p.function_call.name,
                                                                          tool_result={
                                                                              "status": "success",
                                                                              "result": "hello world!",
                                                                          }, extra_fields={})
                                            ],
                                            is_in_progress=False
                                        )

                                        self.conversations.append(interaction)

                                        keep_working = True

                                yield interaction
                    except Exception as e:
                        yield Interaction(
                            role="model",
                            contents=[TextSessionPart(text=f"Error: {str(e)}", extra_fields={})],
                            is_in_progress=False
                        )

                    interaction.is_in_progress = False
