package io.github.devngho.seoah.audio

import io.github.devngho.seoah.config.ConfigProvider
import kotlinx.coroutines.future.await
import java.io.ByteArrayOutputStream
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.util.UUID

/**
 * Transcribes with a Whisper server running on this machine: whisper.cpp's `whisper-server`,
 * or anything that takes the same upload (OpenAI-style `/v1/audio/transcriptions` servers do).
 * With `asr.server` set, it starts `whisper-server` itself.
 */
class WhisperASR(
    val configProvider: ConfigProvider
): ASRProvider {
    private val http = HttpClient.newHttpClient()

    /** whisper-server answers its index page once the model is loaded. */
    private val server by lazy { configProvider.config.asr.server?.let { LocalServer(it, readyPath = "/") } }

    override suspend fun transcribe(pcm: ByteArray): String {
        val config = configProvider.config.asr
        val boundary = "seoah-${UUID.randomUUID()}"

        val fields = buildMap {
            put("response_format", "text")
            put("temperature", "0")
            put("language", config.language)
            config.model?.let { put("model", it) }
        }

        val body = ByteArrayOutputStream().apply {
            for ((name, value) in fields) {
                write("--$boundary\r\nContent-Disposition: form-data; name=\"$name\"\r\n\r\n$value\r\n".toByteArray())
            }
            write(
                ("--$boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"audio.wav\"\r\n" +
                    "Content-Type: audio/wav\r\n\r\n").toByteArray()
            )
            write(wav(pcm, ASRProvider.SAMPLE_RATE))
            write("\r\n--$boundary--\r\n".toByteArray())
        }

        val url = server?.let { it.url() + "/inference" } ?: config.url

        val request = HttpRequest.newBuilder(URI(url))
            .timeout(Duration.ofSeconds(30))
            .header("Content-Type", "multipart/form-data; boundary=$boundary")
            .POST(HttpRequest.BodyPublishers.ofByteArray(body.toByteArray()))
            .build()

        val response = http.sendAsync(request, HttpResponse.BodyHandlers.ofString()).await()
        check(response.statusCode() == 200) { "Whisper server answered ${response.statusCode()}: ${response.body().take(200)}" }

        return response.body().trim().takeUnless { NON_SPEECH.matches(it) }.orEmpty()
    }

    companion object {
        /** What Whisper writes for audio without speech, e.g. `[BLANK_AUDIO]` or `(music)`. */
        private val NON_SPEECH = Regex("""[\[(][^\])]*[\])]""")
    }
}
