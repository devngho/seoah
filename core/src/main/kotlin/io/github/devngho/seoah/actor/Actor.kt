package io.github.devngho.seoah.actor

/**
 * An `Actor` represents how users interact with the sessions: Discord bot for instance.
 */
interface Actor {
    suspend fun run()
}
