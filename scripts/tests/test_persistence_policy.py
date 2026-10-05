"""Changed writers, DDL/startup, foreign schemas and associated files fail closed."""
import importlib.util
from pathlib import Path
import shutil
import sqlite3
import tempfile
import unittest
import subprocess
import sys
import json,os,copy
from unittest.mock import patch
ROOT = Path(__file__).parents[2]
def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module
policy = load("linkup_persistence", ROOT / "scripts/persistence-policy.py")
rehearsal = load("linkup_rehearsal", ROOT / "scripts/image-rehearsal.py")

class PersistenceTests(unittest.TestCase):
    def test_schema_startup_auth_and_new_writer_changes_require_manual_review(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ("internal", "cmd", "release"):
                shutil.copytree(ROOT / name, root / name)
            policy.verify(root)
            for name in ("internal/database/schema.go", "cmd/linkup/main.go", "internal/services/auth_revocation.go", "internal/services/new-writer.go"):
                path = root / name;before = path.read_bytes() if path.exists() else None
                path.write_text("changed writer")
                with self.subTest(name=name), self.assertRaises(ValueError):policy.verify(root)
                path.unlink() if before is None else path.write_bytes(before)

    def test_partial_foreign_and_altered_schema_require_manual_migration(self):
        for sql in ("CREATE TABLE foreign_store(id)", "CREATE TABLE links(id TEXT)"):
            db = sqlite3.connect(":memory:");db.execute(sql)
            with self.subTest(sql=sql), self.assertRaises(ValueError):rehearsal.schema(db)
            db.close()

    def test_associated_file_or_symlink_requires_coordinated_backup(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name in ("linkup.db", "linkup.db-wal", "linkup.db-shm"):(root/name).touch()
            rehearsal.inventory(root)
            (root/"asset.png").touch()
            with self.assertRaises(ValueError):rehearsal.inventory(root)
            (root/"asset.png").unlink();(root/"unexpected").symlink_to(root/"missing")
            with self.assertRaises(ValueError):rehearsal.inventory(root)

    def test_publication_is_blocked_until_signed_fixed_bootstrap(self):
        result=subprocess.run([sys.executable,str(ROOT/"scripts/persistence-policy.py"),"--publication"],capture_output=True,text=True)
        self.assertNotEqual(result.returncode,0)
        self.assertIn("not authorized",result.stderr)

    def test_bootstrap_requires_reviewed_manifest_not_flags_or_environment(self):
        with patch.dict(os.environ,{'BOOTSTRAP_PUBLICATION_AUTHORIZED':'true','GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup'},clear=True):
            with self.assertRaisesRegex(ValueError,'not authorized'):policy.publication()

    def test_exact_reviewed_version_package_and_lock_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'release').mkdir()
            data=policy.bootstrap_policy();data.update(publication_authorized=True,version='0.8.101')
            manifest=root/'release/bootstrap-policy.json';manifest.write_text(json.dumps(data))
            (root/'package.json').write_text(json.dumps({'version':'0.8.101'}))
            (root/'package-lock.json').write_text(json.dumps({'version':'0.8.101','packages':{'':{'version':'0.8.101'}}}))
            good={'GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup'}
            with patch.object(policy,'verify'),patch.dict(os.environ,good,clear=True):
                self.assertEqual(policy.publication(root),'0.8.101')
                with patch.dict(os.environ,{'GITHUB_REF':'refs/tags/v0.8.102'}):
                    with self.assertRaises(ValueError):policy.publication(root)
                (root/'package-lock.json').write_text(json.dumps({'version':'0.8.101','packages':{'':{'version':'0.8.100'}}}))
                with self.assertRaises(ValueError):policy.publication(root)
            for change in ({'automatic_return':True},{'floating_tags':True},{'schema':True},{'publication_authorized':'true'},{'version':None},{'version':'0.8.0'},{'version':'0.8.1-rc.1'},{'version':0.81},{'unexpected':True}):
                manifest.write_text(json.dumps(dict(data,**change)))
                with self.subTest(change=change),self.assertRaises(ValueError):policy.bootstrap_policy(root)
