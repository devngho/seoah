package io.github.devngho.seoah.config

interface ConfigProvider {
    val config: Config

    suspend fun reload()
}