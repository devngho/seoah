package io.github.devngho.seoah.tool

/**
 * Something the model can call. Register an implementation in the Koin module (`bind Tool::class`) to expose it.
 */
interface Tool {
    val name: String
    val description: String

    /** JSON Schema of the arguments, or null when the tool takes none. */
    val parameters: Map<String, Any?>? get() = null

    /**
     * Runs the tool. A call is never interrupted once started, so bound any waiting with a timeout.
     * A thrown exception is reported to the model as an error result.
     */
    suspend fun call(args: Map<String, Any?>): Map<String, Any?>
}
