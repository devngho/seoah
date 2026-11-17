package io.github.devngho.seoah.actor

import io.github.devngho.seoah.audio.TTSProvider
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import net.dv8tion.jda.api.audio.AudioReceiveHandler
import net.dv8tion.jda.api.audio.AudioSendHandler
import net.dv8tion.jda.api.audio.UserAudio
import net.dv8tion.jda.api.entities.User
import java.io.ByteArrayOutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.ConcurrentLinkedQueue
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.TimeSource

/**
 * One voice connection: cuts what each user says into utterances, and plays PCM back.
 * Discord sends every speaker as a separate stream, so each utterance already knows who said it.
 */
class DiscordVoice : AudioReceiveHandler, AudioSendHandler {
    private class Utterance {
        val pcm = ByteArrayOutputStream()
        var lastHeard = TimeSource.Monotonic.markNow()
    }

    private val speaking = HashMap<User, Utterance>()

    override fun canReceiveUser() = true

    override fun handleUserAudio(userAudio: UserAudio) {
        if (userAudio.user.isBot) return

        val data = userAudio.getAudioData(1.0)

        synchronized(speaking) {
            speaking.getOrPut(userAudio.user, ::Utterance).apply {
                pcm.write(data)
                lastHeard = TimeSource.Monotonic.markNow()
            }
        }
    }

    /**
     * Calls [handle] with each finished utterance, as 16 kHz mono 16-bit little-endian PCM.
     * An utterance ends when its speaker has been silent for a moment (Discord sends nothing during silence).
     */
    suspend fun utterances(handle: suspend (User, ByteArray) -> Unit): Nothing {
        while (true) {
            delay(100.milliseconds)

            val finished = synchronized(speaking) {
                speaking
                    .filterValues { it.lastHeard.elapsedNow() > SILENCE || it.pcm.size() >= MAX_UTTERANCE_BYTES }
                    .also { speaking.keys -= it.keys }
            }

            for ((user, utterance) in finished) {
                // shorter than this is a click or a cough, not speech
                if (utterance.pcm.size() >= MIN_UTTERANCE_BYTES) handle(user, toAsrPcm(utterance.pcm.toByteArray()))
            }
        }
    }

    private val frames = ConcurrentLinkedQueue<ByteArray>()
    private var nextFrame: ByteArray? = null

    override fun canProvide(): Boolean {
        nextFrame = frames.poll()
        return nextFrame != null
    }

    override fun provide20MsAudio(): ByteBuffer? = nextFrame?.let { ByteBuffer.wrap(it) }

    /**
     * Plays [speech] (48 kHz mono 16-bit little-endian PCM) as it arrives, and returns once it has all been
     * played. False when there was nothing to play. Cancelling stops the playback at once.
     */
    suspend fun play(speech: Flow<ByteArray>): Boolean {
        var played = false

        try {
            var pending = ByteArray(0)

            speech.collect { chunk ->
                pending += chunk

                val whole = pending.size / SPEECH_FRAME_BYTES * SPEECH_FRAME_BYTES
                for (start in 0 until whole step SPEECH_FRAME_BYTES) frames += toDiscordFrame(pending, start)
                if (whole > 0) played = true

                pending = pending.copyOfRange(whole, pending.size)
            }

            // the last frame is padded with silence
            if (pending.isNotEmpty()) {
                frames += toDiscordFrame(pending.copyOf(SPEECH_FRAME_BYTES), 0)
                played = true
            }

            while (frames.isNotEmpty()) delay(20.milliseconds)
        } finally {
            frames.clear()
        }

        return played
    }

    /** 20 ms of mono little-endian speech, as the stereo big-endian frame Discord takes. */
    private fun toDiscordFrame(speech: ByteArray, start: Int): ByteArray {
        val frame = ByteArray(FRAME_BYTES)

        for (i in 0 until SPEECH_FRAME_BYTES / 2) {
            val low = speech[start + i * 2]
            val high = speech[start + i * 2 + 1]

            frame[i * 4] = high
            frame[i * 4 + 1] = low
            frame[i * 4 + 2] = high
            frame[i * 4 + 3] = low
        }

        return frame
    }

    companion object {
        private const val BYTES_PER_SECOND = 48_000 * 2 * 2
        private const val FRAME_BYTES = BYTES_PER_SECOND / 50
        private const val SPEECH_FRAME_BYTES = TTSProvider.SAMPLE_RATE * 2 / 50
        private const val MIN_UTTERANCE_BYTES = BYTES_PER_SECOND * 3 / 10
        private const val MAX_UTTERANCE_BYTES = BYTES_PER_SECOND * 30
        private val SILENCE = 700.milliseconds
    }
}

/** Discord's 48 kHz stereo big-endian PCM to the 16 kHz mono little-endian PCM that ASR takes. */
internal fun toAsrPcm(discordPcm: ByteArray): ByteArray {
    val input = ByteBuffer.wrap(discordPcm).order(ByteOrder.BIG_ENDIAN).asShortBuffer()
    val samples = input.remaining() / 6 // 2 channels x 3 frames (48 kHz / 16 kHz) per output sample
    val output = ByteBuffer.allocate(samples * 2).order(ByteOrder.LITTLE_ENDIAN)

    repeat(samples) { i ->
        var sum = 0
        repeat(6) { sum += input[i * 6 + it] }
        output.putShort((sum / 6).toShort())
    }

    return output.array()
}
