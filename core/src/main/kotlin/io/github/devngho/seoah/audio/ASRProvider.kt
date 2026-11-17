package io.github.devngho.seoah.audio

interface ASRProvider {
    /**
     * Transcribes one utterance, given as 16 kHz mono 16-bit little-endian PCM.
     * Returns an empty string when nothing intelligible was said.
     */
    suspend fun transcribe(pcm: ByteArray): String

    companion object {
        const val SAMPLE_RATE = 16_000
    }
}
