package io.github.devngho.seoah.config

import io.github.oshai.kotlinlogging.Level
import kotlinx.serialization.Serializable

@Serializable
data class Config(
    val logging: LoggingConfig,
    val lmConfig: LMConfig,
    val discord: DiscordConfig,
    val asr: ASRConfig = ASRConfig(),
    val tts: TTSConfig = TTSConfig()
)

@Serializable
data class LoggingConfig(
    val level: Level
)

@Serializable
data class LMConfig(
    val provider: String,
    val apiKey: String,
    val model: String,
    val prompt: String = "You are a helpful assistant.",
    val etc: Map<String, String> = emptyMap()
)

@Serializable
data class ASRConfig(
    /** "gemini", or "whisper" for a local Whisper server. */
    val provider: String = "gemini",
    /** Whisper only: where a server that is already running takes uploads. Unused when [server] is set. */
    val url: String = "http://127.0.0.1:8080/inference",
    /** Whisper only: how to start `whisper-server` ourselves. */
    val server: ServerConfig? = null,
    /** Whisper only: a language code such as "ko", or "auto" to detect it. */
    val language: String = "auto",
    /** Whisper only: the model name, for servers that serve several. */
    val model: String? = null
)

@Serializable
data class TTSConfig(
    /** "none", or "cosyvoice-cpp" for a local cosyvoice.cpp server. */
    val provider: String = "none",
    /** cosyvoice.cpp only: the address of a server that is already running. Unused when [server] is set. */
    val url: String = "http://127.0.0.1:8080",
    /** cosyvoice.cpp only: that server's `--api-key`, if it has one. */
    val apiKey: String? = null,
    /** cosyvoice.cpp only: how to start `cosyvoice-server` ourselves. */
    val server: ServerConfig? = null,
    /** cosyvoice.cpp only: the server's `--served-model-name`. */
    val model: String = "seoah-v3",
    /** cosyvoice.cpp only: a voice the server was given with `--voice-prompt`. */
    val voice: String = "seoah",
    /**
     * cosyvoice.cpp only: play audio while it is still being synthesized. Streaming costs the server several
     * times the work, so turn it off where synthesis is slower than speech: the audio then arrives whole.
     */
    val stream: Boolean = true
)

/** A server for the bot to start itself. It picks the port and adds `--host` and `--port` to [command]. */
@Serializable
data class ServerConfig(
    /** The program and its arguments, e.g. the model to load. A leading `~/` is the home directory. */
    val command: List<String>,
    val workingDir: String? = null,
    val startupTimeoutSeconds: Int = 120
)

@Serializable
data class DiscordConfig(
    val token: String,
    val debounceMs: Long = 2500,
)