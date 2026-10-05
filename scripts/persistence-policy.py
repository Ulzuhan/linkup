#!/usr/bin/env python3
"""Frozen production Go source: a changed writer/DDL/startup requires manual review."""
import hashlib,json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
BASELINE_SOURCE="0f9d99a9099e33375e336092e38d7b1740e8b865"
REVIEWED_SOURCE="linkup-auth-readiness-v2"
def verify(root=ROOT):
 policy=json.loads((root/"release/persistence-policy.json").read_text())
 if policy.get("baseline_source")!=BASELINE_SOURCE or policy.get("reviewed_source")!=REVIEWED_SOURCE or policy.get("data_action")!="image-only":raise ValueError("manual persistence/return review required")
 paths={p.relative_to(root).as_posix() for folder in ("internal","cmd") for p in (root/folder).rglob("*.go") if not p.name.endswith("_test.go")}
 if paths!=set(policy["files"]):raise ValueError("production Go file set changed: manual migration review")
 for name,digest in policy["files"].items():
  p=root/name
  if p.is_symlink() or hashlib.sha256(p.read_bytes()).hexdigest()!=digest:raise ValueError("production behavior/persistence changed: manual review: "+name)
if __name__=="__main__":
 import sys
 try:
  verify()
  if "--publication" in sys.argv:raise ValueError("publication blocked: fixed signed auth/readiness baseline needs supervised bootstrap")
  print("linkup-sqlite-v1: reviewed auth/readiness correction; old automatic return blocked")
 except (ValueError,OSError,KeyError) as error:print(str(error),file=sys.stderr);sys.exit(1)
