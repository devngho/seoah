package io.github.devngho.seoah.config

import com.akuleshov7.ktoml.Toml
import com.akuleshov7.ktoml.TomlInputConfig
import kotlinx.serialization.decodeFromString
import java.io.File

class TOMLConfigProvider: ConfigProvider {
    private var reloadRequired = false
    private var loadedConfig: Config? = null
    private val toml = Toml(
        inputConfig = TomlInputConfig(
            ignoreUnknownNames = true
        )
    )

    override val config: Config
        get() = synchronized(this) {
            if (loadedConfig == null || reloadRequired) {
                val file = File("../config.toml")
                if (!file.exists()) {
                    throw IllegalStateException("Config file not found: ${file.absolutePath}")
                }

                loadedConfig = file.readText().let { tomlString ->
                    try {
                        toml.decodeFromString<Config>(tomlString)
                    } catch (e: Exception) {
                        throw IllegalStateException("Failed to parse config file: ${file.absolutePath}", e)
                    }
                }
                reloadRequired = false
            }

            loadedConfig!!
        }

    override suspend fun reload() {
        synchronized(this) {
            reloadRequired = true
        }
    }
}