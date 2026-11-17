package io.github.devngho.seoah.audio

import io.github.devngho.seoah.config.ConfigProvider
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.channelFlow
import kotlinx.coroutines.future.await
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.io.IOException
import java.io.InputStream
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.time.Duration
import java.util.UUID

/**
 * Speaks with a cosyvoice.cpp server running on this machine (`cosyvoice-server --api`),
 * through its OpenAI-style `/v1/audio/speech` endpoint. With `tts.server` set, it starts that server itself.
 */
class CosyVoiceTTS(
    val configProvider: ConfigProvider
): TTSProvider {
    private val http = HttpClient.newHttpClient()

    // Our own server gets a random key, so a request can never reach some other service on a reused port.
    private val ownKey = UUID.randomUUID().toString()
    private val server by lazy {
        configProvider.config.tts.server?.let { LocalServer(it, readyPath = "/v1/models", extraArgs = listOf("--api-key", ownKey)) }
    }

    // one synthesis at a time, like the old bot: the server has a single model to run them on
    private val lock = Mutex()

    override fun synthesize(text: String): Flow<ByteArray> = channelFlow {
        if (text.isBlank()) return@channelFlow

        lock.withLock {
            val config = configProvider.config.tts

            val body = buildJsonObject {
                put("model", config.model)
                put("voice", config.voice)
                put("input", text)
                // wav: a header naming the format, then the audio (as it is synthesized, when streaming)
                put("response_format", "wav")
                put("stream", config.stream)
            }

            val server = server
            val url = server?.url() ?: config.url.trimEnd('/')
            val apiKey = if (server != null) ownKey else config.apiKey

            val request = HttpRequest.newBuilder(URI("$url/v1/audio/speech"))
                .timeout(Duration.ofSeconds(180))
                .header("Content-Type", "application/json")
                .apply { apiKey?.let { header("Authorization", "Bearer $it") } }
                .POST(HttpRequest.BodyPublishers.ofString(body.toString()))
                .build()

            val response = http.sendAsync(request, HttpResponse.BodyHandlers.ofInputStream()).await()
            val stream = response.body()

            val reader = launch(Dispatchers.IO) {
                try {
                    check(response.statusCode() == 200) {
                        "cosyvoice.cpp server answered ${response.statusCode()}: ${String(stream.readNBytes(200))}"
                    }

                    val format = readWavFormat(stream)
                    val resampler = Resampler(format.sampleRate, TTSProvider.SAMPLE_RATE)

                    while (true) {
                        // blocks until the chunk is full or the stream ends, so samples are never split
                        val chunk = stream.readNBytes(CHUNK_BYTES)
                        if (chunk.size < format.bytesPerSample) break

                        val whole = chunk.copyOf(chunk.size / format.bytesPerSample * format.bytesPerSample)
                        send(resampler.resample(if (format.float) floatToPcm16(whole) else whole))
                    }
                } catch (e: IOException) {
                    // closing the stream to cancel makes the read fail; that is the cancellation, not an error
                    ensureActive()
                    throw e
                }
            }

            // The read blocks; closing the stream is what unblocks it on cancellation.
            stream.use { reader.join() }
        }
    }

    private class WavFormat(val sampleRate: Int, val float: Boolean) {
        val bytesPerSample = if (float) 4 else 2
    }

    /**
     * Reads up to where the audio starts. The server answers mono, as 16-bit PCM when streaming
     * and as 32-bit float otherwise.
     */
    private fun readWavFormat(stream: InputStream): WavFormat {
        fun read(bytes: Int): ByteBuffer {
            val data = stream.readNBytes(bytes)
            check(data.size == bytes) { "cosyvoice.cpp server answered with a truncated wav" }
            return ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN)
        }

        check(String(read(12).array(), 0, 4) == "RIFF") { "cosyvoice.cpp server did not answer with wav" }

        var format: WavFormat? = null

        while (true) {
            val chunk = read(8)
            val name = String(chunk.array(), 0, 4)
            val size = chunk.getInt(4)

            when (name) {
                "data" -> return checkNotNull(format) { "cosyvoice.cpp server answered with a wav without a format" }
                "fmt " -> {
                    val fmt = read(size)
                    val encoding = fmt.getShort(0).toInt()
                    val bits = fmt.getShort(14).toInt()

                    check(fmt.getShort(2).toInt() == 1 && (encoding == 1 && bits == 16 || encoding == 3 && bits == 32)) {
                        "cosyvoice.cpp server answered with wav it should not: encoding $encoding, $bits bits, ${fmt.getShort(2)} channels"
                    }
                    format = WavFormat(fmt.getInt(4), float = encoding == 3)
                }
                else -> read(size + size % 2) // chunks are padded to an even size
            }
        }
    }

    private fun floatToPcm16(floats: ByteArray): ByteArray {
        val input = ByteBuffer.wrap(floats).order(ByteOrder.LITTLE_ENDIAN).asFloatBuffer()
        val output = ByteBuffer.allocate(input.remaining() * 2).order(ByteOrder.LITTLE_ENDIAN)

        while (input.hasRemaining()) output.putShort((input.get().coerceIn(-1f, 1f) * Short.MAX_VALUE).toInt().toShort())

        return output.array()
    }

    companion object {
        private const val CHUNK_BYTES = 2400
    }
}
