# voice-trainer

Create voice samples for training a styled TTS model.

## How It Works

1. The user provides a corpus and a voice style reference.
   - Corpus could be generated from LLMs, supports Gemini in default.
2. The system generates a set of voice samples that match the provided style using large TTS models that supports natural language instructions.
   - It supports Qwen3 TTS models and OmniVoice for reference-based augmentation.
3. The user picks the best samples.
4. Using the selected samples, the system generates additional samples to augment the dataset by cloning the style of the selected samples.
5. The generated samples are then used to train a smaller, more efficient TTS model that can produce styled speech in real-time.

## Why

- We had to create realtime, styled TTS models without reference audio
- To ensure consistency of results and low latency, we can't use large models like `Qwen/Qwen3-TTS-12Hz-1.7B-VoiceDesign` directly
- We also wanted to avoid some issues such as high costs and copyright concerns
