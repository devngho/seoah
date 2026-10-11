package io.github.devngho.seoah.session

interface SessionItem {
    /** Indicates whether this session item is bound to a specific provider. */
    val isProviderSpecific: Boolean
}

data class UserMessage(
    val content: String
) : SessionItem {
    override val isProviderSpecific: Boolean = false
}

data class ModelMessage(
    val content: String
) : SessionItem {
    override val isProviderSpecific: Boolean = false
}