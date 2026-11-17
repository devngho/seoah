package io.github.devngho.seoah.actor

import io.github.devngho.seoah.audio.wav
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class DiscordVoiceTest {
    @Test
    fun `converts Discord audio to 16 kHz mono little-endian`() {
        // one output sample = 3 stereo frames: left 300, right 100
        val discord = ByteBuffer.allocate(12 * 2).order(ByteOrder.BIG_ENDIAN)
        repeat(6) { discord.putShort(300).putShort(100) }

        val asr = ByteBuffer.wrap(toAsrPcm(discord.array())).order(ByteOrder.LITTLE_ENDIAN).asShortBuffer()

        assertEquals(2, asr.remaining())
        assertEquals(200, asr[0])
        assertEquals(200, asr[1])
    }

    @Test
    fun `wav header describes the audio`() {
        val header = ByteBuffer.wrap(wav(ByteArray(32_000), 16_000)).order(ByteOrder.LITTLE_ENDIAN)

        assertEquals(44 + 32_000, header.capacity())
        assertEquals(36 + 32_000, header.getInt(4))
        assertEquals(1, header.getShort(22)) // mono
        assertEquals(16_000, header.getInt(24))
        assertEquals(32_000, header.getInt(40))
    }

    @Test
    fun `plays audio in 20 ms frames and returns when it is all played`() = runTest {
        val voice = DiscordVoice()
        // 20 ms and a bit of speech (mono, little-endian), arriving in two uneven chunks
        val speech = ByteBuffer.allocate(1920 + 50).order(ByteOrder.LITTLE_ENDIAN)
        repeat((1920 + 50) / 2) { speech.putShort(0x0102) }
        val playing = launch { voice.play(flowOf(speech.array().copyOf(1000), speech.array().copyOfRange(1000, 1970))) }
        runCurrent()

        assertTrue(voice.canProvide())
        val first = voice.provide20MsAudio()!!
        assertEquals(3840, first.remaining())
        // stereo, big-endian
        assertEquals(listOf<Byte>(1, 2, 1, 2), List(4) { first[it] })
        assertTrue(playing.isActive)

        assertTrue(voice.canProvide())
        val last = voice.provide20MsAudio()!!
        assertEquals(3840, last.remaining()) // padded with silence
        assertEquals(0, last[3839])

        playing.join()
        assertFalse(voice.canProvide())
    }

    @Test
    fun `nothing to play is reported`() = runTest {
        assertFalse(DiscordVoice().play(emptyFlow()))
    }

    @Test
    fun `cancelling playback drops what is left`() = runTest {
        val voice = DiscordVoice()
        val playing = launch { voice.play(flowOf(ByteArray(1920 * 10))) }
        runCurrent()
        playing.cancelAndJoin()

        assertFalse(voice.canProvide())
    }
}
