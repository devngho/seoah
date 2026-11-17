package io.github.devngho.seoah.session

import io.github.devngho.seoah.lm.LMChunk
import io.github.devngho.seoah.lm.LMProvider
import io.github.devngho.seoah.tool.Tool
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue

/** Replays one scripted step per request, one chunk per second. */
private class ScriptedLM(private vararg val steps: List<LMChunk>) : LMProvider {
    override val name = "scripted"
    val requests = mutableListOf<List<SessionItem>>()

    override fun stream(conversation: List<SessionItem>, tools: List<Tool>): Flow<LMChunk> = flow {
        requests += conversation
        steps[requests.size - 1].forEach { delay(1000); emit(it) }
    }
}

private class SlowTool : Tool {
    override val name = "slow"
    override val description = "takes ten seconds"
    var finished = false

    override suspend fun call(args: Map<String, Any?>): Map<String, Any?> {
        delay(10_000)
        finished = true
        return mapOf("status" to "done")
    }
}

private val signature = Opaque("scripted", mapOf("s" to "1"))
private val slowCall = ModelPart.ToolCall("call-1", "slow", emptyMap(), signature)

class SessionTest {
    @Test
    fun `sends sentences and commits the finished reply with its signatures`() = runTest {
        val session = Session(ScriptedLM(listOf(LMChunk.Text("Hi! How "), LMChunk.Text("are you?!"), LMChunk.Text("", signature))))
        val sent = mutableListOf<String>()

        session.receive(UserMessage("hello"))
        session.respond { sent += it }

        assertEquals(listOf("Hi!", "How are you?!"), sent)
        assertEquals(
            listOf(UserMessage("hello"), ModelMessage(listOf(ModelPart.Text("Hi! How are you?!"), ModelPart.Text("", signature)))),
            session.history.value
        )
    }

    @Test
    fun `an interrupted reply keeps only what was delivered, before later messages`() = runTest {
        val session = Session(ScriptedLM(listOf(LMChunk.Text("One. "), LMChunk.Text("Two. "), LMChunk.Text("Three. ", signature), LMChunk.Text("Four."))))
        val sent = mutableListOf<String>()

        session.receive(UserMessage("count"))
        val reply = launch { session.respond { sent += it } }
        advanceTimeBy(2500)
        session.receive(UserMessage("stop"))
        reply.cancelAndJoin()

        assertEquals(listOf("One.", "Two."), sent)
        assertEquals(
            listOf(UserMessage("count"), ModelMessage(listOf(ModelPart.Text("One. Two."))), UserMessage("stop")),
            session.history.value
        )
    }

    @Test
    fun `runs a tool call and continues with its result`() = runTest {
        val lm = ScriptedLM(listOf(LMChunk.Text("Wait."), LMChunk.ToolCall(slowCall)), listOf(LMChunk.Text("Done.")))
        val session = Session(lm, listOf(SlowTool()))
        val sent = mutableListOf<String>()
        val notified = mutableListOf<String>()

        session.receive(UserMessage("go"))
        session.respond(onToolCall = { notified += it.name }) { sent += it }

        val result = ToolResult("call-1", "slow", mapOf("status" to "done"))
        val firstStep = ModelMessage(listOf(ModelPart.Text("Wait."), slowCall))

        assertEquals(listOf("Wait.", "Done."), sent)
        assertEquals(listOf("slow"), notified)
        assertEquals(listOf(UserMessage("go"), firstStep, result), lm.requests[1])
        assertEquals(lm.requests[1] + ModelMessage(listOf(ModelPart.Text("Done."))), session.history.value)
    }

    @Test
    fun `a running tool call cannot be interrupted`() = runTest {
        val tool = SlowTool()
        val lm = ScriptedLM(listOf(LMChunk.ToolCall(slowCall)), listOf(LMChunk.Text("Done.")))
        val session = Session(lm, listOf(tool))

        session.receive(UserMessage("go"))
        val reply = launch { session.respond { } }
        advanceTimeBy(5000) // the tool is half-way
        val cancel = launch { reply.cancelAndJoin() }
        runCurrent()

        assertTrue(cancel.isActive, "cancellation must wait for the tool")
        cancel.join()

        assertTrue(tool.finished)
        assertEquals(1, lm.requests.size, "no further model step after the interruption")
        assertEquals(3, session.history.value.size)
        assertIs<ToolResult>(session.history.value.last())
    }

    @Test
    fun `an interruption before the step ends drops its tool call`() = runTest {
        val tool = SlowTool()
        val session = Session(ScriptedLM(listOf(LMChunk.Text("Wait. "), LMChunk.ToolCall(slowCall), LMChunk.Text("more"))), listOf(tool))

        session.receive(UserMessage("go"))
        val reply = launch { session.respond { } }
        advanceTimeBy(2500)
        reply.cancelAndJoin()

        assertEquals(false, tool.finished)
        assertEquals(listOf(UserMessage("go"), ModelMessage(listOf(ModelPart.Text("Wait.")))), session.history.value)
    }

    @Test
    fun `unknown and failing tools report errors to the model`() = runTest {
        val failing = object : Tool {
            override val name = "slow"
            override val description = "fails"
            override suspend fun call(args: Map<String, Any?>): Map<String, Any?> = error("boom")
        }
        val unknown = ModelPart.ToolCall("call-2", "nope", emptyMap())
        val session = Session(ScriptedLM(listOf(LMChunk.ToolCall(slowCall), LMChunk.ToolCall(unknown)), listOf(LMChunk.Text("Sorry."))), listOf(failing))

        session.receive(UserMessage("go"))
        session.respond { }

        assertEquals(
            listOf(ToolResult("call-1", "slow", mapOf("error" to "boom")), ToolResult("call-2", "nope", mapOf("error" to "Unknown tool: nope"))),
            session.history.value.filterIsInstance<ToolResult>()
        )
    }
}
