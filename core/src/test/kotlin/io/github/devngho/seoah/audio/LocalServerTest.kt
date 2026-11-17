package io.github.devngho.seoah.audio

import com.sun.net.httpserver.HttpServer
import io.github.devngho.seoah.config.ServerConfig
import kotlinx.coroutines.runBlocking
import java.net.InetSocketAddress
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import kotlin.test.Test
import kotlin.test.assertContains
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotEquals

/** Stands in for a model server: `--host <host> --port <port> [--load-ms <n>] [--api-key <key>]`. */
fun main(args: Array<String>) {
    fun option(name: String) = args.indexOf(name).takeIf { it >= 0 }?.let { args[it + 1] }

    Thread.sleep(option("--load-ms")?.toLong() ?: 0)

    HttpServer.create(InetSocketAddress(option("--host"), option("--port")!!.toInt()), 0).apply {
        createContext("/ready") { exchange ->
            val body = "key=${option("--api-key")}".toByteArray()
            exchange.sendResponseHeaders(200, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }
        createContext("/exit") { Runtime.getRuntime().halt(0) }
        start()
    }
}

class LocalServerTest {
    private val http = HttpClient.newHttpClient()

    private fun fakeServer(vararg args: String, timeout: Int = 30) = ServerConfig(
        listOf(
            ProcessHandle.current().info().command().get(), "-cp", System.getProperty("java.class.path"),
            "io.github.devngho.seoah.audio.LocalServerTestKt", *args
        ),
        startupTimeoutSeconds = timeout
    )

    private fun get(url: String): String =
        http.send(HttpRequest.newBuilder(URI(url)).build(), HttpResponse.BodyHandlers.ofString()).body()

    @Test
    fun `starts the server and waits until it answers`(): Unit = runBlocking {
        val server = LocalServer(fakeServer("--load-ms", "500"), "/ready", listOf("--api-key", "secret"))

        val url = server.url()

        assertEquals("key=secret", get("$url/ready"))
        assertEquals(url, server.url(), "a running server is reused")
        runCatching { get("$url/exit") }
    }

    @Test
    fun `restarts a server that died`(): Unit = runBlocking {
        val server = LocalServer(fakeServer(), "/ready")
        val first = server.url()

        runCatching { get("$first/exit") }
        Thread.sleep(500)
        val second = server.url()

        assertNotEquals(first, second)
        assertEquals("key=null", get("$second/ready"))
        runCatching { get("$second/exit") }
    }

    @Test
    fun `a server that exits fails the start`(): Unit = runBlocking {
        val server = LocalServer(ServerConfig(listOf("false")), "/ready")

        assertContains(assertFailsWith<IllegalStateException> { server.url() }.message!!, "exited with code 1")
    }

    @Test
    fun `a server that never becomes ready fails the start`(): Unit = runBlocking {
        val server = LocalServer(fakeServer("--load-ms", "60000", timeout = 2), "/ready")

        assertContains(assertFailsWith<IllegalStateException> { server.url() }.message!!, "not ready within 2 s")
    }
}
