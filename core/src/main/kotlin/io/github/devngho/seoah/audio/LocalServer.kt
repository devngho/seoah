package io.github.devngho.seoah.audio

import io.github.devngho.seoah.config.ServerConfig
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.future.await
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import java.io.File
import java.net.ServerSocket
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

/**
 * A model server this bot runs for itself: started on first use on a free local port, restarted if it dies,
 * and stopped when the bot exits. The command gets `--host 127.0.0.1 --port <port>` and [extraArgs] appended.
 *
 * @param readyPath a path the server only answers once its model is loaded
 */
class LocalServer(
    private val config: ServerConfig,
    private val readyPath: String,
    private val extraArgs: List<String> = emptyList()
) {
    private val logger = KotlinLogging.logger {}
    private val http = HttpClient.newHttpClient()

    private val lock = Mutex()
    private var process: Process? = null
    private var url = ""
    private var ready = false

    init {
        Runtime.getRuntime().addShutdownHook(Thread { process?.destroy() })
    }

    /** The server's base URL, once it is up and ready to answer. */
    suspend fun url(): String = lock.withLock {
        if (process?.isAlive != true) start()
        // also reached when an earlier caller was cancelled while waiting
        if (!ready) awaitReady()
        url
    }

    private suspend fun start() {
        val port = ServerSocket(0).use { it.localPort }
        val command = config.command.map(::expandHome) + listOf("--host", "127.0.0.1", "--port", port.toString()) + extraArgs

        logger.info { "Starting ${command.first()} on port $port" }
        ready = false
        url = "http://127.0.0.1:$port"
        process = withContext(Dispatchers.IO) {
            ProcessBuilder(command)
                .apply { config.workingDir?.let { directory(File(expandHome(it))) } }
                .redirectInput(ProcessBuilder.Redirect.from(File("/dev/null")))
                // its logs go to ours
                .redirectOutput(ProcessBuilder.Redirect.INHERIT)
                .redirectError(ProcessBuilder.Redirect.INHERIT)
                .start()
        }
    }

    private suspend fun awaitReady() {
        val process = checkNotNull(process)
        val request = HttpRequest.newBuilder(URI(url + readyPath)).timeout(Duration.ofSeconds(1)).build()

        val answered = withTimeoutOrNull(config.startupTimeoutSeconds.seconds) {
            while (true) {
                check(process.isAlive) { "${config.command.first()} exited with code ${process.exitValue()}; see its log above" }

                val status = try {
                    http.sendAsync(request, HttpResponse.BodyHandlers.discarding()).await().statusCode()
                } catch (_: java.io.IOException) {
                    null // not listening yet
                }
                if (status != null && status < 500) break

                delay(100.milliseconds)
            }
        }

        if (answered == null) {
            process.destroy()
            error("${config.command.first()} was not ready within ${config.startupTimeoutSeconds} s")
        }

        logger.info { "${config.command.first()} is ready at $url" }
        ready = true
    }

    private fun expandHome(path: String) =
        if (path.startsWith("~/")) System.getProperty("user.home") + path.drop(1) else path
}
