package io.github.devngho.seoah.audio

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.emptyFlow

interface TTSProvider {
    /**
     * Speaks [text], streaming 48 kHz mono 16-bit little-endian PCM as it is synthesized.
     * Empty when there is nothing to play. Cold; cancelling the collector stops the synthesis.
     */
    fun synthesize(text: String): Flow<ByteArray>

    companion object {
        const val SAMPLE_RATE = 48_000
    }
}

/** No TTS: replies stay text-only. */
class NoTTS : TTSProvider {
    override fun synthesize(text: String): Flow<ByteArray> = emptyFlow()
}
