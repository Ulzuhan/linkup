#!/usr/bin/env python3
"""Frozen production Go source: a changed writer/DDL/startup requires manual review."""
import hashlib,json,os,re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
BASELINE_SOURCE="0f9d99a9099e33375e336092e38d7b1740e8b865"
REVIEWED_SOURCE="linkup-auth-readiness-v2"
BOOTSTRAP_LABELS={"io.kaicorp.linkup.deployment-lane":"supervised-bootstrap-v1",
                  "io.kaicorp.linkup.automatic-return":"false"}
COMPATIBLE_LABELS={"io.kaicorp.linkup.deployment-lane":"compatible-image-return-v1",
                   "io.kaicorp.linkup.automatic-return":"true"}
def bootstrap_policy(root=ROOT):
 data=json.loads((root/"release/bootstrap-policy.json").read_text())
 if (set(data)!={"schema","lane","publication_authorized","version","automatic_return","floating_tags"}
     or type(data["schema"]) is not int or data["schema"]!=1
     or data["lane"] not in ("supervised-bootstrap-v1","compatible-image-return-v1")
     or type(data["publication_authorized"]) is not bool
     or data["automatic_return"] is not (data["lane"]=="compatible-image-return-v1") or data["floating_tags"] is not False
     or (data["version"] is not None and (type(data["version"]) is not str or not re.fullmatch(r"0\.8\.[1-9][0-9]*",data["version"])))
     or ((data["publication_authorized"] or data["automatic_return"]) and data["version"] is None)):
  raise ValueError("invalid reviewed release/return policy")
 return data
def release_labels(root=ROOT):
 data=bootstrap_policy(root)
 labels=dict(COMPATIBLE_LABELS if data["automatic_return"] else BOOTSTRAP_LABELS)
 labels["org.opencontainers.image.version"]=release_version(root)
 if data["automatic_return"]:
  baseline=json.loads((root/"release/rollback.json").read_text())
  if (baseline.get("version")!="0.8.1" or not re.fullmatch(r"sha256:[a-f0-9]{64}",baseline.get("digest",""))
      or int(data["version"].split(".")[-1])<=1):raise ValueError("reviewed B1 return required")
  labels["io.kaicorp.linkup.rollback-image"]="ghcr.io/ulzuhan/linkup@"+baseline["digest"]
 return labels
def release_version(root=ROOT):
 path=root/"VERSION"
 if path.is_symlink() or not path.is_file():raise ValueError("regular release VERSION file required")
 version=path.read_text().strip()
 if not re.fullmatch(r"0\.8\.[1-9][0-9]*",version) or version!=bootstrap_policy(root)["version"]:
  raise ValueError("VERSION differs from the reviewed bootstrap tag")
 return version
def publication(root=ROOT):
 verify(root)
 data=bootstrap_policy(root)
 if not data["publication_authorized"]:raise ValueError("publication blocked: release publication is not authorized")
 release_version(root)
 if (os.environ.get("GITHUB_REPOSITORY")!="Ulzuhan/linkup"
     or os.environ.get("GITHUB_EVENT_NAME")!="push"
     or os.environ.get("GITHUB_REF")!="refs/tags/v"+data["version"]):
  raise ValueError("publication outside the exact reviewed bootstrap tag")
 return data["version"]
def verify(root=ROOT):
 policy=json.loads((root/"release/persistence-policy.json").read_text())
 if policy.get("baseline_source")!=BASELINE_SOURCE or policy.get("reviewed_source")!=REVIEWED_SOURCE or policy.get("data_action")!="image-only" or policy.get("automatic_return") is not False:raise ValueError("manual persistence/return review required")
 paths={p.relative_to(root).as_posix() for folder in ("internal","cmd") for p in (root/folder).rglob("*.go") if not p.name.endswith("_test.go")}
 if paths!=set(policy["files"]):raise ValueError("production Go file set changed: manual migration review")
 for name,digest in policy["files"].items():
  p=root/name
  if p.is_symlink() or hashlib.sha256(p.read_bytes()).hexdigest()!=digest:raise ValueError("production behavior/persistence changed: manual review: "+name)
if __name__=="__main__":
 import sys
 try:
  verify()
  bootstrap_policy()
  if sys.argv[1:]==["--publication"]:publication()
  elif sys.argv[1:]==["--labels"]:
   print("\n".join(name+"="+value for name,value in release_labels().items()))
  elif sys.argv[1:]:raise ValueError("unsupported policy arguments")
  else:print("linkup-sqlite-v1: frozen auth/readiness behavior; "+bootstrap_policy()["lane"]+" "+str(bootstrap_policy()["version"])+"; publication_authorized="+str(bootstrap_policy()["publication_authorized"]).lower())
 except (ValueError,OSError,KeyError) as error:print(str(error),file=sys.stderr);sys.exit(1)
