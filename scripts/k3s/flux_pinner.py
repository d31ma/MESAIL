import json
import re
import sys
from pathlib import Path


class FluxPinner:
    SEPARATOR: str = "\n---\n"
    COMPONENT: str = r"app\.kubernetes\.io/component: (\S+)"
    KEPT_COMPONENTS: tuple[str, ...] = ("kustomize-controller", "source-controller")
    TAG_IMAGE: str = r"image: ghcr\.io/fluxcd/\S+:v"
    TAG_LINE: str = r"image: ghcr\.io/fluxcd/%s:v\S+"

    def __init__(self, images: dict[str, str]) -> None:
        self.images: dict[str, str] = images

    @classmethod
    def from_pins(cls, pins_path: Path) -> "FluxPinner":
        pins: dict = json.loads(pins_path.read_text())
        return cls(pins["flux"]["images"])

    def pin(self, text: str) -> str:
        kept: str = self.SEPARATOR.join(self._kept_documents(text))
        pinned: str = self._replace_tags(kept)
        self._require_no_tags(pinned)
        return pinned

    def _kept_documents(self, text: str) -> list[str]:
        return [document for document in text.split(self.SEPARATOR) if self._is_kept(document)]

    def _is_kept(self, document: str) -> bool:
        component = re.search(self.COMPONENT, document)
        if component is None:
            return True
        return component.group(1) in self.KEPT_COMPONENTS

    def _replace_tags(self, text: str) -> str:
        for name, digest in self.images.items():
            text = re.sub(self.TAG_LINE % re.escape(name), "image: " + digest, text)
        return text

    def _require_no_tags(self, text: str) -> None:
        if re.search(self.TAG_IMAGE, text):
            raise ValueError("A Flux image is not pinned by digest; nothing written")

    @classmethod
    def main(cls, arguments: list[str]) -> None:
        pinner: FluxPinner = cls.from_pins(Path(arguments[0]))
        sys.stdout.write(pinner.pin(Path(arguments[1]).read_text()))


if __name__ == "__main__":
    FluxPinner.main(sys.argv[1:])
