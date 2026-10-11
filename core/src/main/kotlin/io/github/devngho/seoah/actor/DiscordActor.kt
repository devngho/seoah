package io.github.devngho.seoah.actor
import dev.minn.jda.ktx.jdabuilder.intents
import dev.minn.jda.ktx.jdabuilder.light
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.lm.LMProvider
import io.github.devngho.seoah.session.Session
import net.dv8tion.jda.api.entities.Activity
import net.dv8tion.jda.api.requests.GatewayIntent

class DiscordActor(
    val configProvider: ConfigProvider,
    val lmProvider: LMProvider
): Actor {
    val jda by lazy {
        light(configProvider.config.discord.token, enableCoroutines = true) {
            setActivity(configProvider.config.lmConfig.model.let {
                Activity.customStatus(it)
            })

            intents += GatewayIntent.MESSAGE_CONTENT
        }
    }

    override suspend fun run() {
        println("kawaii seoah-chan")

        jda.awaitShutdown()
    }

    override suspend fun setupSession(session: Session) {
        TODO()
    }
}