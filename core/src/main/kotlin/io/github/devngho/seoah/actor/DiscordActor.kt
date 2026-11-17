package io.github.devngho.seoah.actor
import club.minnced.discord.jdave.interop.JDaveSessionFactory
import dev.minn.jda.ktx.coroutines.await
import dev.minn.jda.ktx.events.listener
import dev.minn.jda.ktx.jdabuilder.intents
import dev.minn.jda.ktx.jdabuilder.light
import io.github.devngho.seoah.audio.ASRProvider
import io.github.devngho.seoah.audio.TTSProvider
import io.github.devngho.seoah.audio.wav
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.lm.LMProvider
import io.github.devngho.seoah.session.Attachment
import io.github.devngho.seoah.session.Session
import io.github.devngho.seoah.session.UserMessage
import io.github.devngho.seoah.tool.Tool
import io.github.oshai.kotlinlogging.KotlinLogging
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancelChildren
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.channels.SendChannel
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.consumeAsFlow
import kotlinx.coroutines.flow.mapNotNull
import kotlinx.coroutines.future.await
import kotlinx.coroutines.launch
import kotlinx.coroutines.runInterruptible
import kotlinx.coroutines.withContext
import net.dv8tion.jda.api.audio.AudioModuleConfig
import net.dv8tion.jda.api.entities.Activity
import net.dv8tion.jda.api.entities.Guild
import net.dv8tion.jda.api.entities.Message
import net.dv8tion.jda.api.entities.User
import net.dv8tion.jda.api.entities.channel.middleman.AudioChannel
import net.dv8tion.jda.api.entities.channel.middleman.MessageChannel
import net.dv8tion.jda.api.events.message.MessageReceivedEvent
import net.dv8tion.jda.api.requests.GatewayIntent
import net.dv8tion.jda.api.utils.FileUpload
import net.dv8tion.jda.api.utils.MemberCachePolicy
import net.dv8tion.jda.api.utils.cache.CacheFlag
import java.io.ByteArrayOutputStream
import java.util.concurrent.ConcurrentHashMap
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

class DiscordActor(
    val configProvider: ConfigProvider,
    val lmProvider: LMProvider,
    val tools: List<Tool>,
    val asr: ASRProvider,
    val tts: TTSProvider
): Actor {
    /** A message on its way into a session, and how long to wait for follow-ups before replying to it. */
    private class Incoming(val message: UserMessage, val debounce: Duration)

    private class VoiceConnection(val channelId: Long, val voice: DiscordVoice, val listening: Job)

    private val logger = KotlinLogging.logger {}

    val jda by lazy {
        light(configProvider.config.discord.token, enableCoroutines = true) {
            setActivity(configProvider.config.lmConfig.model.let {
                Activity.customStatus(it)
            })

            intents += GatewayIntent.MESSAGE_CONTENT

            // voice: who is in which channel, and Discord's end-to-end encryption
            intents += GatewayIntent.GUILD_VOICE_STATES
            enableCache(CacheFlag.VOICE_STATE)
            setMemberCachePolicy(MemberCachePolicy.VOICE)
            setAudioModuleConfig(AudioModuleConfig().withDaveSessionFactory(JDaveSessionFactory()))
        }
    }

    /** Messages are loaded (images downloaded, speech transcribed) in the order they arrived. */
    private val inboxes = ConcurrentHashMap<Long, SendChannel<suspend () -> Incoming?>>()
    private val disabledChannels = ConcurrentHashMap.newKeySet<Long>()

    /** Name → id of everyone seen so far, so a name the model writes can be turned back into a mention. */
    private val userIds = ConcurrentHashMap<String, Long>()

    /** By guild: Discord allows one voice connection per guild. */
    private val voiceConnections = ConcurrentHashMap<Long, VoiceConnection>()

    override suspend fun run() = coroutineScope {
        println("kawaii seoah-chan")

        jda.listener<MessageReceivedEvent> { event ->
            if (event.author.idLong == event.jda.selfUser.idLong) return@listener

            val channel = event.channel

            when (event.message.contentRaw) {
                "[seoah off]" -> {
                    logger.info { "Disabling seoah in channel ${channel.name} (${channel.id})" }
                    disabledChannels += channel.idLong
                    return@listener
                }
                "[seoah on]" -> {
                    logger.info { "Enabling seoah in channel ${channel.name} (${channel.id})" }
                    disabledChannels -= channel.idLong
                    return@listener
                }
                "[seoah join]" -> {
                    joinVoice(event)
                    return@listener
                }
                "[seoah leave]" -> {
                    if (event.isFromGuild) leaveVoice(event.guild)
                    return@listener
                }
            }

            if (channel.idLong in disabledChannels) return@listener

            val message = event.message
            val debounce = configProvider.config.discord.debounceMs.milliseconds

            inbox(channel).send { Incoming(message.toUserMessage(), debounce) }
        }

        runInterruptible(Dispatchers.IO) { jda.awaitShutdown() }
        coroutineContext.cancelChildren()
    }

    private fun CoroutineScope.inbox(channel: MessageChannel): SendChannel<suspend () -> Incoming?> =
        inboxes.computeIfAbsent(channel.idLong) { conversation(channel) }

    private fun CoroutineScope.conversation(channel: MessageChannel): SendChannel<suspend () -> Incoming?> {
        val inbox = Channel<suspend () -> Incoming?>(Channel.UNLIMITED)
        val session = Session(lmProvider, tools)

        launch {
            // any new message cancels the running reply and restarts the debounce
            inbox.consumeAsFlow().mapNotNull { it() }.collectLatest { incoming ->
                session.receive(incoming.message)
                delay(incoming.debounce)

                try {
                    reply(channel, session)
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    logger.error(e) { "Failed to respond in ${channel.name} (${channel.id})" }
                }
            }
        }

        return inbox
    }

    private suspend fun reply(channel: MessageChannel, session: Session) = coroutineScope {
        val typing = launch {
            while (true) {
                channel.sendTyping().await()
                delay(5.seconds)
            }
        }

        try {
            var sent = false

            session.respond(
                onToolCall = { channel.sendMessage("`함수를 호출함: ${it.name} ${it.args}`").await() }
            ) { sentence ->
                // looked up per sentence: the bot may join or leave mid-reply
                val voice = voiceConnections.values.find { it.channelId == channel.idLong }?.voice

                // In voice the sentence is spoken first and written after: one cut off mid-speech is not
                // delivered, so it isn't written. Elsewhere its audio is attached to the message, like the old bot.
                val spoken = voice != null && speech(false) { voice.play(tts.synthesize(sentence)) }
                val recording = if (voice == null) speech(null) { record(sentence) } else null

                // pause before sending, not after: cancelling the pause must not lose a sentence that was sent
                if (sent && !spoken) delay(500.milliseconds)

                val messages = sentence.withMentions().chunked(MAX_MESSAGE_LENGTH)
                messages.forEachIndexed { index, text ->
                    val message = channel.sendMessage(text)
                    if (recording != null && index == messages.lastIndex) {
                        message.addFiles(FileUpload.fromData(recording, "output.wav"))
                    }
                    message.await()
                }
                sent = true
            }
        } finally {
            typing.cancel()
        }
    }

    /** Runs a TTS step. A reply must not fail because its speech did, so errors fall back to [fallback]. */
    private suspend fun <T> speech(fallback: T, block: suspend () -> T): T =
        try {
            block()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            logger.warn(e) { "Failed to synthesize speech" }
            fallback
        }

    /** [sentence] spoken, as a wav file; null when the TTS has nothing to say. */
    private suspend fun record(sentence: String): ByteArray? {
        val pcm = ByteArrayOutputStream()
        tts.synthesize(sentence).collect { pcm.write(it) }

        return pcm.toByteArray().takeIf { it.isNotEmpty() }?.let { wav(it, TTSProvider.SAMPLE_RATE) }
    }

    /** Joins the voice channel whose chat the command was sent in, or else the one its author is in. */
    private fun CoroutineScope.joinVoice(event: MessageReceivedEvent) {
        val audioChannel = event.channel as? AudioChannel ?: event.member?.voiceState?.channel
        // a voice channel's own chat is where its conversation is written
        val chat = audioChannel as? MessageChannel

        if (audioChannel == null || chat == null) {
            event.channel.sendMessage("`음성 채널을 찾을 수 없음`").queue()
            return
        }

        val guild = audioChannel.guild
        leaveVoice(guild)

        val voice = DiscordVoice()
        val listening = launch {
            voice.utterances { user, pcm ->
                if (chat.idLong !in disabledChannels) inbox(chat).send { transcribe(user, pcm) }
            }
        }

        logger.info { "Joining voice channel ${audioChannel.name} (${audioChannel.id})" }
        voiceConnections[guild.idLong] = VoiceConnection(audioChannel.idLong, voice, listening)
        guild.audioManager.apply {
            receivingHandler = voice
            sendingHandler = voice
            openAudioConnection(audioChannel)
        }
    }

    private fun leaveVoice(guild: Guild) {
        val connection = voiceConnections.remove(guild.idLong) ?: return

        logger.info { "Leaving voice channel ${connection.channelId}" }
        connection.listening.cancel()
        guild.audioManager.apply {
            closeAudioConnection()
            receivingHandler = null
            sendingHandler = null
        }
    }

    /** Speech needs no debounce: the silence that ended the utterance was the wait. */
    private suspend fun transcribe(user: User, pcm: ByteArray): Incoming? =
        try {
            userIds[user.name] = user.idLong
            asr.transcribe(pcm).takeIf { it.isNotBlank() }
                ?.let { Incoming(UserMessage("${user.name} said: $it"), Duration.ZERO) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            logger.warn(e) { "Failed to transcribe ${user.name}" }
            null
        }

    private suspend fun Message.toUserMessage(): UserMessage {
        val files = attachments
            .filter { it.contentType != null && it.size <= MAX_ATTACHMENT_BYTES }
            .filter { it.isImage || it.contentType!!.startsWith("audio/") }
            .mapNotNull { file ->
                try {
                    val stream = file.proxy.download().await()
                    // without parameters: Discord may send e.g. "audio/ogg; codecs=opus"
                    Attachment(file.contentType!!.substringBefore(';'), withContext(Dispatchers.IO) { stream.use { it.readBytes() } })
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    logger.warn(e) { "Failed to download ${file.fileName}" }
                    null
                }
            }

        (mentions.users + author + listOfNotNull(referencedMessage?.author)).forEach { userIds[it.name] = it.idLong }

        return UserMessage("${author.name}${replyNote()} said: ${contentWithNames()}", files)
    }

    /** " (replying to name: "quote")" when this message is a reply, else empty. */
    private fun Message.replyNote(): String {
        val replied = referencedMessage ?: return ""
        val quote = replied.contentWithNames().take(MAX_QUOTE_LENGTH)
        return if (quote.isEmpty()) " (replying to ${replied.author.name})"
        else " (replying to ${replied.author.name}: \"$quote\")"
    }

    /** The message's text, with mentions written by the same username its author goes by in "said". */
    private fun Message.contentWithNames(): String {
        var text = contentRaw
        mentions.users.forEach { text = text.replace("<@${it.id}>", "@${it.name}").replace("<@!${it.id}>", "@${it.name}") }
        mentions.roles.forEach { text = text.replace("<@&${it.id}>", "@${it.name}") }
        mentions.channels.forEach { text = text.replace("<#${it.id}>", "#${it.name}") }
        mentions.customEmojis.forEach { text = text.replace(it.asMention, ":${it.name}:") }
        return text
    }

    /** Turns @username back into a real mention, for the users seen so far. */
    private fun String.withMentions(): String = MENTION.replace(this) { match ->
        val written = match.groupValues[1]
        val name = written.trimEnd('.') // a sentence may end right after the name
        userIds[name]?.let { "<@$it>${written.removePrefix(name)}" } ?: match.value
    }

    companion object {
        private val MENTION = Regex("@([A-Za-z0-9_.]+)")
        private const val MAX_ATTACHMENT_BYTES = 8 * 1024 * 1024
        private const val MAX_QUOTE_LENGTH = 200
        private const val MAX_MESSAGE_LENGTH = 2000
    }
}
