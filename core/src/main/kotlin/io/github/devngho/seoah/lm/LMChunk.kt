package io.github.devngho.seoah.lm

import io.github.devngho.seoah.session.ModelPart
import io.github.devngho.seoah.session.Opaque

sealed interface LMChunk {
    data class Text(val delta: String, val providerData: Opaque? = null) : LMChunk
    data class Thought(val delta: String, val providerData: Opaque? = null) : LMChunk
    data class ToolCall(val call: ModelPart.ToolCall) : LMChunk
    data class ProviderSpecific(val providerData: Opaque) : LMChunk
}
