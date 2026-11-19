import hashlib
import importlib.util
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("recover-npm-release.py")
SPEC = importlib.util.spec_from_file_location("recovery", SCRIPT)
recovery = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(recovery)


class RecoveryValidationTests(unittest.TestCase):
    def test_checksums_reject_modified_assets(self):
        with tempfile.TemporaryDirectory() as directory:
            asset = Path(directory) / "aide"
            asset.write_bytes(b"original released binary")
            checksum = hashlib.sha256(asset.read_bytes()).hexdigest()
            recovery.verify_digest(asset, checksum)
            asset.write_bytes(b"different binary")
            with self.assertRaisesRegex(ValueError, "Checksum mismatch"):
                recovery.verify_digest(asset, checksum)

    def test_rejects_malformed_checksums(self):
        with self.assertRaisesRegex(ValueError, "Invalid checksum"):
            recovery.verify_digest(Path("not-read"), "invalid")

    def test_rejects_wrong_package_identity_or_repository(self):
        valid = {"name": "@jmylchreest/aide-plugin", "version": "0.2.1",
                 "repository": {"url": "https://github.com/jmylchreest/aide"}}
        recovery.verify_manifest(valid, valid["name"], "0.2.1", "jmylchreest/aide")
        for mutation in ({"version": "0.2.2"}, {"name": "another-package"},
                         {"repository": {"url": "https://github.com/other/aide"}}):
            with self.subTest(mutation=mutation), self.assertRaises(ValueError):
                recovery.verify_manifest(valid | mutation, valid["name"], "0.2.1", "jmylchreest/aide")

    def test_rejects_nonrelease_refs_before_accessing_the_network(self):
        for tag in ("main", "--help", "v0.2.1;exit", "v0.2.1-beta.1", "v00.2.1"):
            result = subprocess.run(
                [sys.executable, str(SCRIPT), "--tag", tag, "--repository", "jmylchreest/aide",
                 "--output", "/unused-recovery-output"], capture_output=True, text=True,
            )
            with self.subTest(tag=tag):
                self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    unittest.main()
