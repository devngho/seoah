package io.github.devngho.seoah.config

import io.github.oshai.kotlinlogging.Level
import kotlinx.serialization.Serializable

@Serializable
data class Config(
    val logging: LoggingConfig,
    val lmConfig: LMConfig
)

@Serializable
data class LoggingConfig(
    val level: Level
)

@Serializable
data class LMConfig(
    val apiKey: String,
    val model: String
)