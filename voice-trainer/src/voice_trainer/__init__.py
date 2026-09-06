import argparse
import asyncio
import json
import os
from pathlib import Path

from voice_trainer.audio import create_audios_from_texts
from voice_trainer.config import load_config, setup_config
from voice_trainer.llm import generate_sentences
from voice_trainer.log import log


async def _gen_corpus(config_path: str | None = None) -> None:
    setup_config(config_path)

    config = load_config()

    if os.path.exists(config.path_to_save_corpus):
        raise FileExistsError()

    log(
        lambda: (
            f"Creating {config.num_sentence_to_generate} using {config.backend_model}."
        ),
        "INFO",
    )

    sentences = await generate_sentences()

    with open(config.path_to_save_corpus, "w") as file:
        file.write("\n".join(sentences))

    log(
        lambda: f"Wrote {len(sentences)} sentences into {config.path_to_save_corpus}.",
        "INFO",
    )


def gen_corpus(config_path: str | None = None) -> None:
    asyncio.run(_gen_corpus(config_path))


def synthesize(config_path: str | None = None) -> None:
    setup_config(config_path)

    config = load_config()

    texts = list(dict.fromkeys(_read_corpus(config.path_to_save_corpus)))
    texts.sort(key=len, reverse=True)

    _save_dataset(
        config.folder_to_save_audio_design, (texts[:config.audio_design_sample_count]) * config.audio_design_candidate_count
    )


def _read_corpus(path: str) -> list[str]:
    texts = [
        line.strip()
        for line in Path(path).read_text(encoding="utf-8").splitlines()
        if line.strip()
    ]
    if not texts:
        raise ValueError("The corpus contains no non-empty sentences.")
    return texts


def _save_dataset(
    folder: str, texts: list[str], references: list[tuple[str, str]] | None = None
) -> None:
    output = Path(folder)
    if output.exists() and any(output.iterdir()):
        raise FileExistsError(f"Output directory is not empty: {output}")
    audios = create_audios_from_texts(texts, references)
    output.mkdir(parents=True, exist_ok=True)
    metadata = []
    samples = {}
    for idx, (text, audio_bytes) in enumerate(audios):
        sample_id = f"{idx:05d}"
        path = output / f"{sample_id}.wav"
        path.write_bytes(audio_bytes)
        samples[sample_id] = {"path": path.name, "text": text}
        metadata.append(
            load_config().audio_metadata_format.format(path=path, text=text)
        )
    (output / "metadata.list").write_text("\n".join(metadata), encoding="utf-8")
    (output / "samples.json").write_text(
        json.dumps(samples, ensure_ascii=False, indent=2), encoding="utf-8"
    )


def augment() -> None:
    """Clone selected VoiceDesign samples over the target corpus."""
    parser = argparse.ArgumentParser(description=augment.__doc__)
    parser.add_argument("--config", default=None, help="Configuration TOML path")
    parser.add_argument(
        "--samples",
        nargs="+",
        required=True,
        help="Design sample IDs, e.g. 00000 00003",
    )
    parser.add_argument(
        "--corpus", help="Target corpus (defaults to configured corpus)"
    )
    args = parser.parse_args()
    setup_config(args.config)
    config = load_config()
    design = Path(config.folder_to_save_audio_design)
    try:
        samples = json.loads((design / "samples.json").read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        parser.error(f"Cannot read design samples; run synthesize first: {exc}")
    references = []
    for sample_id in dict.fromkeys(args.samples):
        if sample_id not in samples:
            parser.error(f"Unknown design sample ID: {sample_id}")
        sample = samples[sample_id]
        path = design / sample["path"]
        if not path.is_file() or not sample["text"].strip():
            parser.error(f"Missing audio or transcript for sample: {sample_id}")
        references.append((str(path.resolve()), sample["text"]))
    _save_dataset(
        config.folder_to_save_audio_clone,
        _read_corpus(args.corpus or config.path_to_save_corpus),
        references,
    )
