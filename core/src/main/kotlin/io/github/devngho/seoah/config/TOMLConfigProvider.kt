package io.github.devngho.seoah.config

import com.akuleshov7.ktoml.Toml
import kotlinx.serialization.decodeFromString
import java.io.File

class TOMLConfigProvider: ConfigProvider {
    private var reloadRequired = false

    override val config: Config by lazy {
        val file = File("config.toml")
        if (!file.exists()) {
            throw IllegalStateException("Config file not found: ${file.absolutePath}")
        }

        val config = file.readText().let { tomlString ->
            try {
                Toml.decodeFromString<Config>(tomlString)
            } catch (e: Exception) {
                throw IllegalStateException("Failed to parse config file: ${file.absolutePath}", e)
            }
        }

        config
    }

    override suspend fun reload() {
        synchronized(this) {
            reloadRequired = true
        }
    }
}