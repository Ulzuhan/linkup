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

    def test_disabled_manifest_blocks_publication_before_context_or_version(self):
        disabled=dict(policy.bootstrap_policy(),publication_authorized=False,version=None)
        with patch.object(policy,'bootstrap_policy',return_value=disabled):
            with self.assertRaisesRegex(ValueError,'not authorized'):policy.publication()

    def test_bootstrap_requires_reviewed_manifest_not_flags_or_environment(self):
        disabled=dict(policy.bootstrap_policy(),publication_authorized=False,version=None)
        with patch.object(policy,'bootstrap_policy',return_value=disabled),patch.dict(os.environ,{'BOOTSTRAP_PUBLICATION_AUTHORIZED':'true','GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup'},clear=True):
            with self.assertRaisesRegex(ValueError,'not authorized'):policy.publication()

    def test_exact_reviewed_go_version_and_canonical_tag_context_required(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'release').mkdir()
            data=policy.bootstrap_policy();data.update(publication_authorized=True,version='0.8.101')
            manifest=root/'release/bootstrap-policy.json';manifest.write_text(json.dumps(data))
            version=root/'VERSION';version.write_text('0.8.101\n')
            good={'GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup'}
            with patch.object(policy,'verify'),patch.dict(os.environ,good,clear=True):
                self.assertEqual(policy.publication(root),'0.8.101')
                for change in ({'GITHUB_REF':'refs/tags/v0.8.102'},{'GITHUB_REF':'refs/heads/main'},{'GITHUB_EVENT_NAME':'pull_request'},{'GITHUB_EVENT_NAME':'workflow_dispatch'},{'GITHUB_REPOSITORY':'fork/linkup'}):
                    with self.subTest(change=change),patch.dict(os.environ,change):
                        with self.assertRaises(ValueError):policy.publication(root)
                for value in ('0.8.100\n','0.8.101\n0.8.102\n','0.8.101-rc.1\n'):
                    version.write_text(value)
                    with self.subTest(value=value),self.assertRaises(ValueError):policy.publication(root)
                version.unlink()
                with self.assertRaises(ValueError):policy.publication(root)
                (root/'target').write_text('0.8.101\n');version.symlink_to(root/'target')
                with self.assertRaises(ValueError):policy.publication(root)
            for change in ({'automatic_return':False},{'floating_tags':True},{'schema':True},{'publication_authorized':'true'},{'version':None},{'version':'0.8.0'},{'version':'0.8.1-rc.1'},{'version':0.81},{'unexpected':True},{'lane':'unknown'}):
                manifest.write_text(json.dumps(dict(data,**change)))
                with self.subTest(change=change),self.assertRaises(ValueError):policy.bootstrap_policy(root)

    def test_compatible_capability_never_authorizes_publication(self):
        data=dict(policy.bootstrap_policy(),publication_authorized=False)
        self.assertTrue(data['automatic_return']);self.assertFalse(data['publication_authorized'])
        self.assertEqual(policy.release_labels()['io.kaicorp.linkup.rollback-image'], 'ghcr.io/ulzuhan/linkup@'+json.loads((ROOT/'release/rollback.json').read_text())['digest'])
        env={'GITHUB_REF':'refs/tags/v'+data['version'],'GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup'}
        with patch.object(policy,'bootstrap_policy',return_value=data),patch.dict(os.environ,env,clear=True),self.assertRaisesRegex(ValueError,'not authorized'):policy.publication()
