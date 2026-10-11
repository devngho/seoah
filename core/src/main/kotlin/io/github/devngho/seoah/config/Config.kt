package io.github.devngho.seoah.config

import io.github.oshai.kotlinlogging.Level
import kotlinx.serialization.Serializable

@Serializable
data class Config(
    val logging: LoggingConfig,
    val lmConfig: LMConfig,
    val discord: DiscordConfig
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
    val etc: Map<String, String> = emptyMap()
)

@Serializable
data class DiscordConfig(
    val token: String,
)