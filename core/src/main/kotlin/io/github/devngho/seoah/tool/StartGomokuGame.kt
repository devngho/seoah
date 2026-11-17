package io.github.devngho.seoah.tool

class StartGomokuGame : Tool {
    override val name = "start_gomoku_game"
    override val description = "Start a new game of Gomoku. You must call this function to play the game."

    override suspend fun call(args: Map<String, Any?>): Map<String, Any?> =
        TODO("Gomoku engine is not connected yet")
}
