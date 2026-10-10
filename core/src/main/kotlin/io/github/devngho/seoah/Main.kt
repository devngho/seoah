package io.github.devngho.seoah

import io.github.devngho.seoah.actor.Actor
import io.github.devngho.seoah.actor.DiscordActor
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.config.TOMLConfigProvider
import org.koin.core.context.GlobalContext.get
import org.koin.core.context.startKoin
import org.koin.dsl.bind
import org.koin.dsl.module

suspend fun main() {
    startKoin {
        module {
            single<TOMLConfigProvider>() bind ConfigProvider::class
            single { DiscordActor() as Actor }
        }
    }

    val actor: Actor = get()
}