package io.github.devngho.seoah.audio

import com.sun.net.httpserver.HttpServer
import io.github.devngho.seoah.config.Config
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.config.DiscordConfig
import io.github.devngho.seoah.config.LMConfig
import io.github.devngho.seoah.config.LoggingConfig
import io.github.devngho.seoah.config.TTSConfig
import io.github.oshai.kotlinlogging.Level
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import java.net.InetSocketAddress
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.CountDownLatch
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class CosyVoiceTTSTest {
    private var status = 200
    private var request = ""
    private var authorization: String? = null

    /** Released to let the server finish its answer. */
    private val finish = CountDownLatch(1)

    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
        createContext("/v1/audio/speech") { exchange ->
            request = exchange.requestBody.readBytes().decodeToString()
            authorization = exchange.requestHeaders.getFirst("Authorization")

            if (status != 200) {
                val body = "voice not found".toByteArray()
                exchange.sendResponseHeaders(status, body.size.toLong())
                exchange.responseBody.use { it.write(body) }
                return@createContext
            }

            if ("\"stream\":false" in request) {
                // what cosyvoice.cpp answers when not streaming: 32-bit float wav, here with an extra chunk
                val audio = ByteBuffer.allocate(2400 * 4).order(ByteOrder.LITTLE_ENDIAN).apply { repeat(2400) { putFloat(0.5f) } }.array()
                val body = ByteBuffer.allocate(12 + 12 + 24 + 8 + audio.size).order(ByteOrder.LITTLE_ENDIAN)
                    .put("RIFF".toByteArray()).putInt(0).put("WAVE".toByteArray())
                    .put("LIST".toByteArray()).putInt(3).put(ByteArray(4))
                    .put("fmt ".toByteArray()).putInt(16).putShort(3).putShort(1).putInt(24_000).putInt(96_000).putShort(4).putShort(32)
                    .put("data".toByteArray()).putInt(audio.size).put(audio)
                    .array()
                exchange.sendResponseHeaders(200, body.size.toLong())
                exchange.responseBody.use { it.write(body) }
                return@createContext
            }

            // what cosyvoice.cpp streams: a wav header of unknown length, then 24 kHz PCM
            exchange.sendResponseHeaders(200, 0)
            exchange.responseBody.use { out ->
                out.write(wav(ByteArray(0), 24_000))
                out.write(pcm(1200, 1000))
                out.flush()
                finish.await()
                out.write(pcm(1200, 1000))
            }
        }
        start()
    }

    private fun tts(stream: Boolean) = CosyVoiceTTS(object : ConfigProvider {
        override val config = Config(
            LoggingConfig(Level.INFO),
            LMConfig("gemini-ai-studio", "key", "model"),
            DiscordConfig("token"),
            tts = TTSConfig("cosyvoice-cpp", "http://127.0.0.1:${server.address.port}/", "secret", stream = stream)
        )

        override suspend fun reload() {}
    })

    private val tts = tts(stream = true)

    private fun pcm(samples: Int, value: Short): ByteArray =
        ByteBuffer.allocate(samples * 2).order(ByteOrder.LITTLE_ENDIAN).apply { repeat(samples) { putShort(value) } }.array()

    @AfterTest
    fun stop() {
        finish.countDown()
        server.stop(0)
    }

    @Test
    fun `streams speech as 48 kHz PCM`() = runBlocking {
        finish.countDown()

        val speech = tts.synthesize("안녕 \"서아\"").toList().reduce(ByteArray::plus)

        assertContains(request, "\"input\":\"안녕 \\\"서아\\\"\"")
        assertContains(request, "\"voice\":\"seoah\"")
        assertContains(request, "\"stream\":true")
        assertEquals("Bearer secret", authorization)

        // 2400 samples at 24 kHz are 0.1 s, so about 4800 samples at 48 kHz
        val samples = ByteBuffer.wrap(speech).order(ByteOrder.LITTLE_ENDIAN).asShortBuffer()
        assertTrue(samples.remaining() in 4790..4800, "got ${samples.remaining()} samples")
        assertTrue((0 until samples.remaining()).all { samples[it] == 1000.toShort() })
    }

    @Test
    fun `without streaming, takes the whole answer as 32-bit float`(): Unit = runBlocking {
        val speech = tts(stream = false).synthesize("hello").toList().reduce(ByteArray::plus)

        val samples = ByteBuffer.wrap(speech).order(ByteOrder.LITTLE_ENDIAN).asShortBuffer()
        assertTrue(samples.remaining() in 4790..4800, "got ${samples.remaining()} samples")
        assertTrue((0 until samples.remaining()).all { samples[it] == (Short.MAX_VALUE / 2).toShort() })
    }

    @Test
    fun `audio arrives before the server has finished`() = runBlocking {
        // the server is still holding back its second half
        val first = withTimeout(5000) { tts.synthesize("hello").first() }

        assertTrue(first.isNotEmpty())
    }

    @Test
    fun `a server error fails the synthesis`(): Unit = runBlocking {
        status = 404

        assertContains(assertFailsWith<IllegalStateException> { tts.synthesize("hello").toList() }.message!!, "voice not found")
    }

    @Test
    fun `resampling keeps its place between chunks`() {
        // a ramp cut into uneven chunks must come out as the same ramp
        val ramp = ByteBuffer.allocate(600 * 2).order(ByteOrder.LITTLE_ENDIAN).apply { repeat(600) { putShort((it * 10).toShort()) } }.array()
        val whole = Resampler(24_000, 48_000).resample(ramp)
        val resampler = Resampler(24_000, 48_000)
        val pieces = listOf(ramp.copyOfRange(0, 14), ramp.copyOfRange(14, 500), ramp.copyOfRange(500, 1200))
            .map { resampler.resample(it) }.reduce(ByteArray::plus)

        assertEquals(whole.toList(), pieces.toList())

        val samples = ByteBuffer.wrap(whole).order(ByteOrder.LITTLE_ENDIAN).asShortBuffer()
        assertEquals(1198, samples.remaining())
        assertEquals(listOf<Short>(0, 5, 10, 15), List(4) { samples[it] })
    }
}
