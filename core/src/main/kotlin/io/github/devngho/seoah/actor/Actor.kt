package io.github.devngho.seoah.actor

import io.github.devngho.seoah.session.Session

/**
 * An `Actor` represents how users interact with the sessions: Discord bot for instance.
 */
interface Actor {
    suspend fun setup()

    suspend fun setupSession(session: Session)
}