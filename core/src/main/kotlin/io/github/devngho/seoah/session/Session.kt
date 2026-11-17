package io.github.devngho.seoah.session

import io.github.devngho.seoah.lm.LMChunk
import io.github.devngho.seoah.lm.LMProvider
import io.github.devngho.seoah.tool.Tool
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.buffer
import kotlinx.coroutines.flow.filterIsInstance
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.withContext

class Session(private val lm: LMProvider, private val tools: List<Tool> = emptyList()) {
    private val _history = MutableStateFlow<List<SessionItem>>(emptyList())
    val history: StateFlow<List<SessionItem>> = _history.asStateFlow()

    fun receive(message: UserMessage) = _history.update { it + message }

    /**
     * Generates the next reply and hands it to [send] sentence by sentence, running any tools the model calls
     * ([onToolCall] is told before each one runs).
     *
     * Cancelling the caller interrupts generation; only sentences [send] returned from are committed.
     * Tool calls are the exception: once the model has asked for them, they run to completion first.
     */
    suspend fun respond(onToolCall: suspend (ModelPart.ToolCall) -> Unit = {}, send: suspend (String) -> Unit) {
        // The reply is inserted here, so messages that arrive mid-reply stay after it.
        var anchor = _history.value.size

        while (true) {
            val parts = generate(anchor, send)
            val calls = parts.filterIsInstance<ModelPart.ToolCall>()

            if (calls.isEmpty()) {
                if (parts.isNotEmpty()) commit(anchor, listOf(ModelMessage(parts)))
                return
            }

            // A call, its side effects and its result must stay together: history with a call but no result
            // (or a tool that ran but isn't in history) is inconsistent. So this can't be interrupted;
            // a pending cancellation takes effect right after.
            anchor = withContext(NonCancellable) {
                val results = calls.map { ToolResult(it.id, it.name, execute(it, onToolCall)) }
                commit(anchor, listOf(ModelMessage(parts)) + results)
            }
            currentCoroutineContext().ensureActive()
        }
    }

    /** Runs one model step. Returns its parts uncommitted; if interrupted, commits the delivered text instead. */
    private suspend fun generate(anchor: Int, send: suspend (String) -> Unit): List<ModelPart> {
        val generated = mutableListOf<ModelPart>()
        val delivered = StringBuilder()

        try {
            lm.stream(_history.value.take(anchor), tools)
                .onEach { generated.append(it) }
                .filterIsInstance<LMChunk.Text>()
                .map { it.delta }
                .sentences()
                .buffer() // generation keeps going while delivery is slow
                .collect { sentence ->
                    sentence.trim().takeIf { it.isNotEmpty() }?.let { send(it) }
                    delivered.append(sentence)
                }
        } catch (e: Throwable) {
            // Signatures and tool calls are dropped: they belong to a step the users never fully saw.
            if (delivered.isNotBlank()) commit(anchor, listOf(ModelMessage(listOf(ModelPart.Text(delivered.toString().trim())))))
            throw e
        }

        return generated
    }

    private suspend fun execute(
        call: ModelPart.ToolCall,
        onToolCall: suspend (ModelPart.ToolCall) -> Unit
    ): Map<String, Any?> {
        val tool = tools.find { it.name == call.name } ?: return mapOf("error" to "Unknown tool: ${call.name}")

        // Telling the actor is best-effort; failing to do so must not stop the tool.
        try { onToolCall(call) } catch (_: Exception) {}

        return try {
            tool.call(call.args)
        } catch (e: Throwable) {
            mapOf("error" to (e.message ?: e.toString()))
        }
    }

    /** Inserts [items] at [anchor] and returns the position right after them. */
    private fun commit(anchor: Int, items: List<SessionItem>): Int {
        _history.update { it.take(anchor) + items + it.drop(anchor) }
        return anchor + items.size
    }
}

/** Merges streamed deltas into parts. A signed chunk stays its own part, since its signature covers only that part. */
private fun MutableList<ModelPart>.append(chunk: LMChunk) {
    val last = lastOrNull()

    when (chunk) {
        is LMChunk.Text ->
            if (last is ModelPart.Text && last.providerData == null && chunk.providerData == null)
                this[lastIndex] = last.copy(text = last.text + chunk.delta)
            else add(ModelPart.Text(chunk.delta, chunk.providerData))
        is LMChunk.Thought ->
            if (last is ModelPart.Thought && last.providerData == null && chunk.providerData == null)
                this[lastIndex] = last.copy(text = last.text + chunk.delta)
            else add(ModelPart.Thought(chunk.delta, chunk.providerData))
        is LMChunk.ToolCall -> add(chunk.call)
        is LMChunk.ProviderSpecific -> add(ModelPart.ProviderSpecific(chunk.providerData))
    }
}

// The lookahead keeps runs like "?!" together.
private val sentenceEnd = Regex("""(?:\. |[!?\n\r])(?![!?])""")

private val GROWING_ENDS = setOf("!", "?")

/** Splits text deltas after ". ", "!", "?" and line breaks. */
private fun Flow<String>.sentences(): Flow<String> = flow {
    var pending = ""

    collect { delta ->
        pending += delta
        while (true) {
            val match = sentenceEnd.find(pending) ?: break
            val end = match.range.last + 1
            // "?" or "!" at the very end may still grow into "?!", so wait for the next delta
            if (end == pending.length && match.value in GROWING_ENDS) break
            emit(pending.take(end))
            pending = pending.drop(end)
        }
    }

    if (pending.isNotEmpty()) emit(pending)
}
