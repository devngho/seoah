"""Standalone worker, executed by CosyVoice's Python 3.10 environment."""

import importlib
import json
import os
import sys
import traceback
from functools import partial
from pathlib import Path

WETEXT_FILES = tuple(
    f"{language}/tn/{name}.fst"
    for language in ("en", "zh")
    for name in ("tagger", "verbalizer")
)


def cached_wetext_snapshot(download, model_id: str) -> str:
    """Use ModelScope's persistent cache without remote revision checks."""
    if model_id != "pengzhendong/wetext":
        return download(model_id)

    def complete(root):
        return all(
            (Path(root) / name).is_file() and (Path(root) / name).stat().st_size > 0
            for name in WETEXT_FILES
        )

    try:
        root = download(model_id, local_files_only=True)
    except (ValueError, OSError):
        root = None
    if root is not None and complete(root):
        return root
    # CosyVoice only uses TN; do not fetch unrelated ITN/Japanese assets.
    root = download(model_id, allow_patterns=list(WETEXT_FILES))
    if not complete(root):
        raise RuntimeError("WeText download did not provide all required TN assets")
    return root


def configure_wetext_cache():
    """Adapt only WeText's downloader; leave the installed SDK untouched."""
    try:
        wetext = importlib.import_module("wetext.wetext")
    except ImportError:
        return  # Optional frontend; CosyVoice has its own fallback.
    vars(wetext)["snapshot_download"] = partial(
        cached_wetext_snapshot, wetext.snapshot_download
    )


def prepare_prompt_text(model_dir: str, prompt_text: str) -> str:
    """Match AutoModel's YAML selection and each generation's tokenizer format."""
    root = Path(model_dir)
    delimiter = "<|endofprompt|>"
    # v1 (300M) and v2 use plain transcripts, not v3's instruction tokens.
    if (root / "cosyvoice.yaml").is_file() or (root / "cosyvoice2.yaml").is_file():
        transcript = prompt_text.split(delimiter, 1)[-1].strip()
    elif (root / "cosyvoice3.yaml").is_file():
        transcript = prompt_text.split(delimiter, 1)[-1].strip()
        if transcript:
            return (
                prompt_text
                if delimiter in prompt_text
                else "You are a helpful assistant." + delimiter + transcript
            )
    else:
        raise ValueError(f"No supported CosyVoice YAML found in {model_dir}")
    if not transcript:
        raise ValueError("CosyVoice reference transcript must not be empty")
    return transcript


def main():
    # The sibling bridge is also called cosyvoice.py; do not shadow the SDK.
    sys.path.remove(os.path.dirname(os.path.abspath(__file__)))
    # Reserve a separate descriptor for protocol replies, including native stdout logs.
    with os.fdopen(os.dup(sys.stdout.fileno()), "w", buffering=1) as protocol:
        os.dup2(sys.stderr.fileno(), sys.stdout.fileno())
        import soundfile as sf
        import torch

        # This dependency lives in the external worker environment, not core's venv.
        from cosyvoice.cli.cosyvoice import (  # pyright: ignore[reportMissingImports]
            AutoModel,
        )

        try:
            settings = json.loads(sys.stdin.readline())
        except (ValueError, OSError):
            traceback.print_exc(file=sys.stderr)
            return
        torch.set_num_threads(settings["threads"])
        prompt_text = prepare_prompt_text(settings["model"], settings["prompt_text"])
        configure_wetext_cache()
        model = AutoModel(model_dir=settings["model"], fp16=False)
        model.add_zero_shot_spk(prompt_text, settings["prompt_wav"], "seoah")
        protocol.write(json.dumps({"ready": True}) + "\n")
        for line in sys.stdin:
            try:
                request = json.loads(line)
                with torch.inference_mode():
                    outputs = model.inference_zero_shot(
                        request["text"],
                        "",
                        "",
                        zero_shot_spk_id="seoah",
                        stream=False,
                        text_frontend=False,
                    )
                    chunks = [item["tts_speech"].detach().cpu() for item in outputs]
                    if not chunks:
                        raise RuntimeError("CosyVoice produced no audio")
                    audio = torch.cat(chunks, dim=1).float().squeeze(0)
                if audio.numel() == 0 or not torch.isfinite(audio).all():
                    raise RuntimeError("CosyVoice produced invalid audio")
                sf.write(
                    request["output"],
                    audio.numpy(),
                    model.sample_rate,
                    format="OGG",
                    subtype="VORBIS",
                )
                reply = {"ok": True}
            except Exception as error:  # noqa: BLE001 -- serialize SDK failures at the process boundary
                traceback.print_exc(file=sys.stderr)
                reply = {"error": str(error)}
            protocol.write(json.dumps(reply) + "\n")


if __name__ == "__main__":
    main()
