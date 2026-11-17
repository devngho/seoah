package io.github.devngho.seoah.lm

import com.google.genai.Client
import com.google.genai.types.Content
import com.google.genai.types.FunctionCall
import com.google.genai.types.FunctionDeclaration
import com.google.genai.types.FunctionResponse
import com.google.genai.types.GenerateContentConfig
import com.google.genai.types.Part
import com.google.genai.types.ThinkingConfig
import com.google.genai.types.Tool as GeminiTool
import com.google.genai.types.ToolConfig
import com.google.genai.types.UrlContext
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.session.ModelMessage
import io.github.devngho.seoah.session.ModelPart
import io.github.devngho.seoah.session.Opaque
import io.github.devngho.seoah.session.SessionItem
import io.github.devngho.seoah.session.ToolResult
import io.github.devngho.seoah.session.UserMessage
import io.github.devngho.seoah.tool.Tool
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.future.asDeferred
import kotlinx.coroutines.launch
import kotlin.jvm.optionals.getOrNull

class GeminiAIS(
    val configProvider: ConfigProvider
): LMProvider {
    override val name: String = "Gemini (AI Studio)"

    private val client by lazy {
        Client.builder()
            .apiKey(configProvider.config.lmConfig.apiKey)
            .build()
    }

    private val generationConfig by lazy {
        val lmConfig = configProvider.config.lmConfig

        GenerateContentConfig.builder()
            .thinkingConfig(
                ThinkingConfig.builder()
                    .thinkingLevel((lmConfig.etc["thinking_effort"] ?: "LOW").uppercase())
                    .includeThoughts(true)
                    .build()
            )
            .systemInstruction(Content.builder().parts(Part.builder().text(lmConfig.prompt).build()).build())
            .build()
    }

    override fun stream(conversation: List<SessionItem>, tools: List<Tool>): Flow<LMChunk> = channelFlow {
        val geminiTools = buildList {
            if (configProvider.config.lmConfig.etc["url_context"] == "true")
                add(GeminiTool.builder().urlContext(UrlContext.builder()).build())
            if (tools.isNotEmpty())
                add(GeminiTool.builder().functionDeclarations(tools.map { it.toDeclaration() }).build())
        }

        val config = if (geminiTools.isEmpty()) generationConfig else generationConfig.toBuilder()
            .tools(geminiTools)
            // needed to mix Gemini's own tools with ours
            .apply { if (geminiTools.size > 1) toolConfig(ToolConfig.builder().includeServerSideToolInvocations(true)) }
            .build()

        val pending = client.async.models
            .generateContentStream(configProvider.config.lmConfig.model, conversation.toContents(), config)

        // Cancelling doesn't recall the request: its response still arrives and has to be closed, or the
        // connection leaks. Awaiting the future directly would cancel it and lose that response.
        val response = try {
            pending.asDeferred().await()
        } catch (e: CancellationException) {
            pending.thenAccept { it.close() }
            throw e
        }

        val reader = launch(Dispatchers.IO) {
            try {
                for (chunk in response) {
                    chunk.parts().orEmpty().forEach { part -> part.toChunk()?.let { send(it) } }
                }
            } catch (e: Exception) {
                // closing the stream to cancel makes the read fail; that is the cancellation, not an error
                ensureActive()
                throw e
            }
        }

        // ResponseStream blocks while reading; closing it is the only way to unblock the reader on cancellation.
        response.use { reader.join() }
    }

    private fun Part.toChunk(): LMChunk? {
        val signature = thoughtSignature().getOrNull()?.let { Opaque(name, mapOf(SIGNATURE to it.toHexString())) }

        functionCall().getOrNull()?.let { call ->
            return LMChunk.ToolCall(
                ModelPart.ToolCall(call.id().getOrNull(), call.name().get(), call.args().getOrNull().orEmpty(), signature)
            )
        }

        // Anything else (server-side tool use, a bare signature) goes back to Gemini exactly as it came.
        val text = text().getOrNull()
            ?: return toJson().takeIf { it != "{}" }?.let { LMChunk.ProviderSpecific(Opaque(name, mapOf(RAW_PART to it))) }

        return if (thought().getOrNull() == true) LMChunk.Thought(text, signature) else LMChunk.Text(text, signature)
    }

    private fun Tool.toDeclaration(): FunctionDeclaration =
        FunctionDeclaration.builder()
            .name(name)
            .description(description)
            .apply { parameters?.let { parametersJsonSchema(it) } }
            .build()

    /**
     * Consecutive items of one kind become a single turn, as the old session did.
     * Tool results stay in a turn of their own, right after the calls they answer.
     */
    private fun List<SessionItem>.toContents(): List<Content> {
        val turns = mutableListOf<Triple<String, Boolean, MutableList<Part>>>()

        for (item in this) {
            val (role, parts) = when (item) {
                is UserMessage -> "user" to
                    listOf(Part.fromText(item.content)) + item.attachments.map { Part.fromBytes(it.data, it.mimeType) }
                is ModelMessage -> "model" to item.parts.mapNotNull { it.toPart() }
                is ToolResult -> "user" to listOf(item.toPart())
            }
            val last = turns.lastOrNull()

            if (last != null && last.first == role && last.second == item is ToolResult) last.third += parts
            else turns += Triple(role, item is ToolResult, parts.toMutableList())
        }

        return turns.map { (role, _, parts) -> Content.builder().role(role).parts(parts).build() }
    }

    private fun ToolResult.toPart(): Part =
        Part.builder()
            .functionResponse(FunctionResponse.builder().name(name).response(result).apply { callId?.let { id(it) } })
            .build()

    /** Null for parts that belong to another provider. */
    private fun ModelPart.toPart(): Part? = when (this) {
        is ModelPart.Text -> Part.builder().text(text).signedWith(providerData).build()
        is ModelPart.Thought -> Part.builder().text(text).thought(true).signedWith(providerData).build()
        is ModelPart.ToolCall -> Part.builder()
            .functionCall(FunctionCall.builder().name(name).args(args).apply { id?.let { id(it) } })
            .signedWith(providerData)
            .build()
        is ModelPart.ProviderSpecific ->
            providerData.takeIf { it.provider == name }?.data?.get(RAW_PART)?.let { Part.fromJson(it) }
    }

    private fun Part.Builder.signedWith(data: Opaque?) = apply {
        data?.takeIf { it.provider == name }?.data?.get(SIGNATURE)?.let { thoughtSignature(it.hexToByteArray()) }
    }

    companion object {
        private const val SIGNATURE = "thought_signature"
        private const val RAW_PART = "part"
    }
}
