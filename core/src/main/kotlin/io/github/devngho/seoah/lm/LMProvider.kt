package io.github.devngho.seoah.lm

import io.github.devngho.seoah.session.SessionItem
import io.github.devngho.seoah.tool.Tool
import kotlinx.coroutines.flow.Flow

interface LMProvider {
    val name: String

    /**
     * Streams the model's next output, which may call any of [tools].
     * Cold; cancelling the collector aborts the request.
     */
    fun stream(conversation: List<SessionItem>, tools: List<Tool> = emptyList()): Flow<LMChunk>
}
