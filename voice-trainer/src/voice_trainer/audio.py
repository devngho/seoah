from io import BytesIO

import numpy as np
import soundfile as sf
from tqdm import trange

from voice_trainer.config import load_config


def _inference_dtype(device_name: str):
    import torch

    device = torch.device(device_name)
    if device.type == "cuda":
        if not torch.cuda.is_available():
            raise RuntimeError(
                "CUDA is unavailable. Install CUDA-enabled PyTorch and check the NVIDIA driver."
            )
        with torch.cuda.device(device):
            return torch.bfloat16 if torch.cuda.is_bf16_supported() else torch.float16
    return torch.float32


def generate_qwen_tts_audios(
    texts: list[str], references: list[tuple[str, str]] | None = None
) -> tuple[list[tuple[str, np.ndarray]], int]:
    import torch
    from qwen_tts import Qwen3TTSModel

    config = load_config()

    if not texts:
        return [], 24000

    dtype = _inference_dtype(config.torch_device)

    model = Qwen3TTSModel.from_pretrained(
        config.audio_design_model if references is None else config.audio_clone_model,
        device_map=config.torch_device,
        dtype=dtype,
        attn_implementation="sdpa",
    )

    audios = []
    sr = 24000
    with torch.inference_mode():
        prompts = None
        if references is not None:
            if not references:
                raise ValueError("Select at least one reference sample.")
            prompts = model.create_voice_clone_prompt(
                ref_audio=[path for path, _ in references],
                # ref_text=[text for _, text in references],
                x_vector_only_mode=True,
            )
        for start in trange(0, len(texts), config.audio_batch_size):
            batch = texts[start : start + config.audio_batch_size]
            if prompts is None:
                wavs, sr = model.generate_voice_design(
                    text=batch,
                    language=[config.audio_lang] * len(batch),
                    instruct=[config.audio_design_prompt] * len(batch),
                )
            else:
                wavs, sr = model.generate_voice_clone(
                    text=batch,
                    language=[config.audio_lang] * len(batch),
                    voice_clone_prompt=[
                        prompts[i % len(prompts)]
                        for i in range(start, start + len(batch))
                    ],
                    non_streaming_mode=True,
                )
            audios.extend(zip(batch, wavs, strict=True))

    return audios, sr


def generate_omnivoice_audios(
    texts: list[str], references: list[tuple[str, str]] | None = None
) -> tuple[list[tuple[str, np.ndarray]], int]:
    """Clone references with OmniVoice, encoding each prompt once per run."""
    if not references:
        raise ValueError(
            "OmniVoice augmentation requires at least one reference sample."
        )
    if not texts:
        return [], 24000

    import torch

    try:
        from omnivoice import OmniVoice
    except ModuleNotFoundError as exc:
        if exc.name != "omnivoice":
            raise
        raise RuntimeError(
            "OmniVoice is not installed. Run augmentation with "
            "`uv run --no-default-groups --extra omnivoice augment ...`."
        ) from exc

    config = load_config()
    model = OmniVoice.from_pretrained(
        config.audio_clone_model,
        device_map=config.torch_device,
        dtype=_inference_dtype(config.torch_device),
    )
    audios = []
    with torch.inference_mode():
        prompts = [
            model.create_voice_clone_prompt(ref_audio=path, ref_text=text)
            for path, text in references
        ]
        for start in trange(0, len(texts), config.audio_batch_size):
            batch = texts[start : start + config.audio_batch_size]
            wavs = model.generate(
                text=batch,
                language=[config.audio_lang] * len(batch),
                voice_clone_prompt=[
                    prompts[i % len(prompts)] for i in range(start, start + len(batch))
                ],
            )
            audios.extend(zip(batch, wavs, strict=True))
    sr = model.sampling_rate
    if not isinstance(sr, int) or sr <= 0:
        raise ValueError("OmniVoice returned an invalid sampling rate.")
    return audios, sr


def create_audios_from_texts(
    texts: list[str], references: list[tuple[str, str]] | None = None
) -> list[tuple[str, bytes]]:
    """Return ordered (text, WAV bytes) pairs from the configured backend."""
    config = load_config()

    model_kind = (
        config.audio_design_model_kind
        if references is None
        else config.audio_clone_model_kind
    )
    if model_kind == "qwen-tts":
        wavs, sr = generate_qwen_tts_audios(texts, references)
    elif model_kind == "omnivoice" and references is not None:
        wavs, sr = generate_omnivoice_audios(texts, references)
    else:
        raise ValueError(f"Unsupported audio model kind: {model_kind}")

    audio_list = []
    for text, wav in wavs:
        with BytesIO() as buffer:
            sf.write(buffer, wav, samplerate=sr, format="WAV")
            audio_list.append((text, buffer.getvalue()))
    return audio_list
