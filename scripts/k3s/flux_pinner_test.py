import unittest

from flux_pinner import FluxPinner


class FluxPinnerTest(unittest.TestCase):
    SOURCE: str = "ghcr.io/fluxcd/source-controller@sha256:" + "a" * 64
    KUSTOMIZE: str = "ghcr.io/fluxcd/kustomize-controller@sha256:" + "b" * 64
    INSTALL: str = (
        "kind: Namespace\nmetadata:\n  name: flux-system\n"
        "\n---\n"
        "kind: Deployment\nmetadata:\n  labels:\n    app.kubernetes.io/component: source-controller\n"
        "spec:\n  image: ghcr.io/fluxcd/source-controller:v1.10.1\n"
        "\n---\n"
        "kind: Deployment\nmetadata:\n  labels:\n    app.kubernetes.io/component: kustomize-controller\n"
        "spec:\n  image: ghcr.io/fluxcd/kustomize-controller:v1.10.1\n"
        "\n---\n"
        "kind: Deployment\nmetadata:\n  labels:\n    app.kubernetes.io/component: helm-controller\n"
        "spec:\n  image: ghcr.io/fluxcd/helm-controller:v1.4.1\n"
    )

    def pinner(self) -> FluxPinner:
        return FluxPinner({"source-controller": self.SOURCE, "kustomize-controller": self.KUSTOMIZE})

    def test_pins_source_controller_by_digest(self) -> None:
        self.assertIn("image: " + self.SOURCE, self.pinner().pin(self.INSTALL))

    def test_pins_kustomize_controller_by_digest(self) -> None:
        self.assertIn("image: " + self.KUSTOMIZE, self.pinner().pin(self.INSTALL))

    def test_drops_other_controllers(self) -> None:
        self.assertNotIn("helm-controller", self.pinner().pin(self.INSTALL))

    def test_keeps_documents_without_component(self) -> None:
        self.assertIn("name: flux-system", self.pinner().pin(self.INSTALL))

    def test_refuses_a_kept_image_without_digest(self) -> None:
        pinner = FluxPinner({"source-controller": self.SOURCE})
        with self.assertRaises(ValueError):
            pinner.pin(self.INSTALL)


if __name__ == "__main__":
    unittest.main()
