package io.github.devngho.seoah.session

sealed interface SessionItem

data class UserMessage(val content: String, val attachments: List<Attachment> = emptyList()) : SessionItem

/** A file sent along with a message, e.g. an image. */
class Attachment(val mimeType: String, val data: ByteArray)

data class ModelMessage(val parts: List<ModelPart>) : SessionItem

/** The outcome of a [ModelPart.ToolCall]; always follows the [ModelMessage] that made the call. */
data class ToolResult(val callId: String?, val name: String, val result: Map<String, Any?>) : SessionItem

sealed interface ModelPart {
    data class Text(val text: String, val providerData: Opaque? = null) : ModelPart
    data class Thought(val text: String, val providerData: Opaque? = null) : ModelPart
    data class ToolCall(
        val id: String?,
        val name: String,
        val args: Map<String, Any?>,
        val providerData: Opaque? = null
    ) : ModelPart

    /** A part only its provider understands (e.g. Gemini's server-side tool use). It is handed back as it came. */
    data class ProviderSpecific(val providerData: Opaque) : ModelPart
}

/** Data bound to one provider (e.g. Gemini thought signatures). Other providers ignore it. */
data class Opaque(val provider: String, val data: Map<String, String>)
