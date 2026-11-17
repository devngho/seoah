package io.github.devngho.seoah.audio

import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlin.math.floor

/** Wraps mono 16-bit little-endian PCM in a WAV header. */
internal fun wav(pcm: ByteArray, sampleRate: Int): ByteArray =
    ByteBuffer.allocate(44 + pcm.size).order(ByteOrder.LITTLE_ENDIAN)
        .put("RIFF".toByteArray()).putInt(36 + pcm.size).put("WAVE".toByteArray())
        .put("fmt ".toByteArray()).putInt(16).putShort(1).putShort(1)
        .putInt(sampleRate).putInt(sampleRate * 2).putShort(2).putShort(16)
        .put("data".toByteArray()).putInt(pcm.size).put(pcm)
        .array()

/**
 * Changes the sample rate of a mono 16-bit little-endian PCM stream by linear interpolation.
 * Feed the chunks in order: it remembers where the previous one ended.
 */
internal class Resampler(from: Int, to: Int) {
    private val step = from.toDouble() / to

    /** Where the next output sample falls, in input samples from the start of the next chunk. */
    private var position = 0.0
    private var previous: Short = 0

    fun resample(chunk: ByteArray): ByteArray {
        if (step == 1.0) return chunk

        val input = ByteBuffer.wrap(chunk).order(ByteOrder.LITTLE_ENDIAN).asShortBuffer()
        val size = input.remaining()
        if (size == 0) return chunk

        fun sample(index: Int) = if (index < 0) previous else input[index]

        val output = ByteBuffer.allocate(((size - position) / step).toInt().coerceAtLeast(0) * 2 + 2)
            .order(ByteOrder.LITTLE_ENDIAN)

        while (true) {
            val index = floor(position).toInt()
            if (index + 1 >= size) break

            val fraction = position - index
            output.putShort((sample(index) + (sample(index + 1) - sample(index)) * fraction).toInt().toShort())
            position += step
        }

        previous = input[size - 1]
        position -= size

        return output.array().copyOf(output.position())
    }
}
