"""Publication contract tests: synthetic skopeo only, no registry or credentials."""
import hashlib
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parents[1] / "publish-scanned-image.sh"
MANIFEST = '{"schemaVersion":2,"synthetic":true}'
DIGEST = "sha256:" + hashlib.sha256(MANIFEST.encode()).hexdigest()
FAKE = '''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open(os.environ["TEST_CALLS"], "a") as stream:
    stream.write(json.dumps(args) + "\\n")
if args[0] == "inspect":
    if os.environ.get("TEST_INSPECT_FAIL"):
        sys.exit(1)
    sys.stdout.write(os.environ["TEST_MANIFEST"])
elif args[0] == "copy":
    if os.environ.get("TEST_COPY_FAIL"):
        sys.exit(1)
    Path(args[args.index("--digestfile") + 1]).write_text(os.environ["TEST_COPY_DIGEST"] + "\\n")
else:
    sys.exit(2)
'''


class PublishTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.base = Path(self.directory.name)
        fake = self.base / "skopeo"
        fake.write_text(FAKE)
        fake.chmod(0o700)
        (self.base / "layout").mkdir()
        (self.base / "layout/index.json").write_text(MANIFEST)
        (self.base / "config.json").write_text('{"auths":{}}')
        self.calls = self.base / "calls.jsonl"
        self.env = dict(os.environ, PATH=str(self.base) + os.pathsep + os.environ["PATH"],
                        DOCKER_CONFIG=str(self.base), TMPDIR=str(self.base),
                        RELEASE_LAYOUT=str(self.base / "layout"), RELEASE_DIGEST=DIGEST,
                        RELEASE_REPOSITORY="ghcr.io/example/linkup",
                        RELEASE_TAGS="ghcr.io/example/linkup:0.5.1\nghcr.io/example/linkup:latest",
                        TEST_CALLS=str(self.calls), TEST_MANIFEST=MANIFEST, TEST_COPY_DIGEST=DIGEST)

    def run_publish(self, *args):
        result = subprocess.run(["bash", str(SCRIPT), *args], env=self.env,
                                capture_output=True, text=True, timeout=10)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()] if self.calls.exists() else []
        return result, calls

    def test_verify_only_never_copies(self):
        result, calls = self.run_publish("--verify-only")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), DIGEST)
        self.assertEqual([c[0] for c in calls], ["inspect"])

    def test_same_layout_and_digest_for_every_tag(self):
        result, calls = self.run_publish()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.strip(), DIGEST)
        self.assertEqual([c[0] for c in calls], ["inspect", "copy", "copy"])
        for call, tag in zip(calls[1:], self.env["RELEASE_TAGS"].splitlines()):
            self.assertIn("--all", call)
            self.assertIn("--preserve-digests", call)
            self.assertEqual(call[-2:], ["oci:" + self.env["RELEASE_LAYOUT"], "docker://" + tag])
            self.assertNotIn("--dest-creds", call)
        self.assertEqual(list(self.base.glob("linkup-publish-digest.*")), [])

    def test_mismatch_never_publishes(self):
        self.env["RELEASE_DIGEST"] = "sha256:" + "0" * 64
        result, calls = self.run_publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([c[0] for c in calls], ["inspect"])

    def test_invalid_destinations_rejected_before_any_copy(self):
        for tags in ("ghcr.io/another/linkup:latest", "ghcr.io/example/linkup:bad tag", "--creds=secret"):
            with self.subTest(tags=tags):
                self.env["RELEASE_TAGS"] = tags
                result, calls = self.run_publish()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])

    def test_failed_inspection_stops_publication(self):
        self.env["TEST_INSPECT_FAIL"] = "1"
        result, calls = self.run_publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([c[0] for c in calls], ["inspect"])

    def test_failed_copy_stops_remaining_tags(self):
        self.env["TEST_COPY_FAIL"] = "1"
        result, calls = self.run_publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([c[0] for c in calls], ["inspect", "copy"])

    def test_changed_destination_digest_stops_remaining_tags(self):
        self.env["TEST_COPY_DIGEST"] = "sha256:" + "1" * 64
        result, calls = self.run_publish()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([c[0] for c in calls], ["inspect", "copy"])


if __name__ == "__main__":
    unittest.main()
