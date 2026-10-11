package io.github.devngho.seoah.lm

import com.google.genai.Client
import io.github.devngho.seoah.config.ConfigProvider
import io.github.devngho.seoah.session.SessionItem
import kotlinx.coroutines.flow.Flow

class GeminiAIS(
    val configProvider: ConfigProvider
): LMProvider {
    override val name: String = "Gemini (AI Studio)"

    private val client by lazy { Client.builder().apiKey(configProvider.config.lmConfig.apiKey).build(); }

    override suspend fun streamResponses(conversation: List<SessionItem>): Flow<String> {
        TODO("Not yet implemented")
    }
}