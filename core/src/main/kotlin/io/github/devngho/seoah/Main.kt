package io.github.devngho.seoah

import io.github.devngho.seoah.actor.Actor
import io.github.devngho.seoah.actor.DiscordActor
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.config.TOMLConfigProvider
import io.github.devngho.seoah.lm.GeminiAIS
import io.github.devngho.seoah.lm.LMProvider
import kotlinx.coroutines.coroutineScope
import org.koin.core.context.startKoin
import org.koin.dsl.bind
import org.koin.dsl.module
import org.koin.mp.KoinPlatform
import org.koin.plugin.module.dsl.single

suspend fun main() = coroutineScope {
    startKoin {
        modules(
            module {
                single<TOMLConfigProvider>() bind ConfigProvider::class
                single<DiscordActor>() bind Actor::class

                single<LMProvider> {
                    when (get<ConfigProvider>().config.lmConfig.provider) {
                        "gemini-ai-studio" -> GeminiAIS(get())
                        else -> error("Unknown provider")
                    }
                }
            }
        )
    }

    val actor: Actor = KoinPlatform.getKoin().get()

    actor.run()
}