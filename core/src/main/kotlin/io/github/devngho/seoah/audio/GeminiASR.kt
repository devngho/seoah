package io.github.devngho.seoah.audio

import com.google.genai.Client
import com.google.genai.types.Content
import com.google.genai.types.Part
import io.github.devngho.seoah.config.ConfigProvider
import kotlinx.coroutines.future.await

/** Transcribes with a Gemini model: `lmConfig.etc.asr_model`, or the chat model when unset. */
class GeminiASR(
    val configProvider: ConfigProvider
): ASRProvider {
    private val client by lazy {
        Client.builder()
            .apiKey(configProvider.config.lmConfig.apiKey)
            .build()
    }

    override suspend fun transcribe(pcm: ByteArray): String {
        val lmConfig = configProvider.config.lmConfig
        val request = Content.fromParts(Part.fromBytes(wav(pcm, ASRProvider.SAMPLE_RATE), "audio/wav"), Part.fromText(PROMPT))

        return client.async.models
            .generateContent(lmConfig.etc["asr_model"] ?: lmConfig.model, request, null)
            .await()
            .text()
            .orEmpty()
            .trim()
    }

    companion object {
        private const val PROMPT =
            "Transcribe the speech in this audio exactly as spoken, in its original language. " +
                "Output only the transcript. If there is no intelligible speech, output nothing."
    }
}
