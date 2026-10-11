package io.github.devngho.seoah.lm

import io.github.devngho.seoah.session.SessionItem
import kotlinx.coroutines.flow.Flow

interface LMProvider {
    val name: String

    suspend fun streamResponses(conversation: List<SessionItem>): Flow<String>
}