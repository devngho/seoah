package io.github.devngho.seoah.actor
import io.github.devngho.seoah.session.Session

class DiscordActor: Actor {
    override suspend fun setup() {
        println("kawaii seoah-chan")
    }

    override suspend fun setupSession(session: Session) {
        TODO()
    }
}