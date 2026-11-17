package io.github.devngho.seoah.audio

import com.sun.net.httpserver.HttpServer
import io.github.devngho.seoah.config.ASRConfig
import io.github.devngho.seoah.config.Config
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.config.DiscordConfig
import io.github.devngho.seoah.config.LMConfig
import io.github.devngho.seoah.config.LoggingConfig
import io.github.oshai.kotlinlogging.Level
import kotlinx.coroutines.runBlocking
import java.net.InetSocketAddress
import kotlin.test.AfterTest
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class WhisperASRTest {
    private var status = 200
    private var answer = ""
    private var upload = ""
    private var contentType = ""

    private val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0).apply {
        createContext("/inference") { exchange ->
            contentType = exchange.requestHeaders.getFirst("Content-Type")
            upload = exchange.requestBody.readBytes().toString(Charsets.ISO_8859_1)
            val body = answer.toByteArray()
            exchange.sendResponseHeaders(status, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }
        start()
    }

    private val asr = WhisperASR(object : ConfigProvider {
        override val config = Config(
            LoggingConfig(Level.INFO),
            LMConfig("gemini-ai-studio", "key", "model"),
            DiscordConfig("token"),
            ASRConfig("whisper", "http://127.0.0.1:${server.address.port}/inference", language = "ko", model = "large-v3")
        )

        override suspend fun reload() {}
    })

    @AfterTest
    fun stop() = server.stop(0)

    @Test
    fun `uploads the utterance as wav and returns the transcript`() = runBlocking {
        answer = " 안녕하세요.\n"

        assertEquals("안녕하세요.", asr.transcribe(ByteArray(3200)))

        val boundary = contentType.substringAfter("boundary=")
        assertContains(upload, "name=\"language\"\r\n\r\nko\r\n")
        assertContains(upload, "name=\"model\"\r\n\r\nlarge-v3\r\n")
        assertContains(upload, "name=\"response_format\"\r\n\r\ntext\r\n")
        assertContains(upload, "name=\"file\"; filename=\"audio.wav\"\r\nContent-Type: audio/wav\r\n\r\nRIFF")
        assertEquals(true, upload.endsWith("\r\n--$boundary--\r\n"))
    }

    @Test
    fun `audio without speech is an empty transcript`() = runBlocking {
        answer = "[BLANK_AUDIO]\n"

        assertEquals("", asr.transcribe(ByteArray(3200)))
    }

    @Test
    fun `a server error fails the transcription`(): Unit = runBlocking {
        status = 500
        answer = "model not loaded"

        assertContains(assertFailsWith<IllegalStateException> { asr.transcribe(ByteArray(3200)) }.message!!, "model not loaded")
    }
}
