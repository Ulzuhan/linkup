"""Changed writers, DDL/startup, foreign schemas and associated files fail closed."""
import importlib.util
from pathlib import Path
import shutil
import sqlite3
import tempfile
import unittest
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
