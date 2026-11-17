package io.github.devngho.seoah

import io.github.devngho.seoah.actor.Actor
import io.github.devngho.seoah.actor.DiscordActor
import io.github.devngho.seoah.audio.ASRProvider
import io.github.devngho.seoah.audio.CosyVoiceTTS
import io.github.devngho.seoah.audio.GeminiASR
import io.github.devngho.seoah.audio.NoTTS
import io.github.devngho.seoah.audio.TTSProvider
import io.github.devngho.seoah.audio.WhisperASR
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.config.TOMLConfigProvider
import io.github.devngho.seoah.lm.GeminiAIS
import io.github.devngho.seoah.lm.LMProvider
import io.github.devngho.seoah.tool.StartGomokuGame
import io.github.devngho.seoah.tool.Tool
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
                single { DiscordActor(get(), get(), getAll<Tool>(), get(), get()) } bind Actor::class

                single<ASRProvider> {
                    when (get<ConfigProvider>().config.asr.provider) {
                        "gemini" -> GeminiASR(get())
                        "whisper" -> WhisperASR(get())
                        else -> error("Unknown ASR provider")
                    }
                }
                single<TTSProvider> {
                    when (get<ConfigProvider>().config.tts.provider) {
                        "none" -> NoTTS()
                        "cosyvoice-cpp" -> CosyVoiceTTS(get())
                        else -> error("Unknown TTS provider")
                    }
                }

                single<StartGomokuGame>() bind Tool::class

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