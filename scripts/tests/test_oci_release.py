"""Synthetic OCI graphs and mocked copiers; no Docker, registry or credentials."""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import sys
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("oci", Path(__file__).parents[1] / "oci-release.py")
oci = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(oci)
SOURCE = "b" * 40


class OCITests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.layout = Path(self.tmp.name)
        (self.layout / "blobs/sha256").mkdir(parents=True)
        labels = {"io.kaicorp.linkup.data-action": "image-only", "org.opencontainers.image.revision": SOURCE, "io.kaicorp.linkup.store-contract": "linkup-sqlite-v1",
                  **oci.policy.BOOTSTRAP_LABELS,
                  "io.kaicorp.linkup.auth-contract": "oidc-subject-v2", "io.kaicorp.linkup.readiness-contract":"sqlite-ro-v1"}
        self.config = self.put({"architecture": "amd64", "os": "linux", "config": {"Labels": labels}})
        layer = self.put(b"synthetic layer")
        runtime = self.put({"config": self.config, "layers": [layer]})
        attestation = self.put({"config": self.put({}), "layers": [
            self.put({"predicateType": "https://slsa.dev/provenance/v0.2"}),
            self.put({"predicateType": "https://spdx.dev/Document"})]})
        runtime["platform"] = {"architecture": "amd64", "os": "linux"}
        attestation["platform"] = {"architecture": "unknown", "os": "unknown"}
        attestation["annotations"] = {"vnd.docker.reference.type": "attestation-manifest"}
        self.root = {"mediaType": "application/vnd.oci.image.index.v1+json", "manifests": [runtime, attestation]}

    def put(self, value):
        data = value if isinstance(value, bytes) else json.dumps(value).encode()
        digest = hashlib.sha256(data).hexdigest()
        (self.layout / "blobs/sha256" / digest).write_bytes(data)
        return {"digest": "sha256:" + digest, "size": len(data)}

    def verify(self, root=None, expected=None, source=SOURCE):
        raw = json.dumps(root or self.root).encode()
        with patch.object(oci, "command", return_value=raw):
            return oci.verify(self.layout, expected or "sha256:" + hashlib.sha256(raw).hexdigest(), source)

    def test_valid_graph_returns_exact_runtime_config_id(self):
        self.assertEqual(self.verify(), self.config["digest"])

    def test_wrong_root_source_or_layer_cannot_pass(self):
        with self.assertRaises(oci.Refused):
            self.verify(expected="sha256:" + "a" * 64)
        with self.assertRaises(oci.Refused):
            self.verify(source="a" * 40)
        path = self.layout / "blobs/sha256" / self.config["digest"].split(":")[1]
        path.write_bytes(b"different bytes")
        with self.assertRaises(oci.Refused):
            self.verify()

    def test_extra_platform_or_missing_attestations_cannot_pass(self):
        root = copy.deepcopy(self.root)
        root["manifests"].append(copy.deepcopy(root["manifests"][0]))
        root["manifests"][-1]["platform"]["architecture"] = "arm64"
        with self.assertRaises(oci.Refused):
            self.verify(root)
        root = copy.deepcopy(self.root)
        root["manifests"].pop()
        with self.assertRaises(oci.Refused):
            self.verify(root)

    def test_old_or_missing_auth_readiness_contracts_are_rejected(self):
        for label in ("io.kaicorp.linkup.auth-contract", "io.kaicorp.linkup.readiness-contract"):
            root=copy.deepcopy(self.root)
            config=json.loads(oci.blob(self.layout,self.config));config["config"]["Labels"].pop(label)
            manifest=json.loads(oci.blob(self.layout,root["manifests"][0]));manifest["config"]=self.put(config)
            descriptor=self.put(manifest);descriptor["platform"]={"architecture":"amd64","os":"linux"}
            root["manifests"][0]=descriptor
            with self.subTest(label=label),self.assertRaises(oci.Refused):self.verify(root)

    def test_external_descriptors_and_symlinks_are_rejected(self):
        descriptor = dict(self.config, urls=["https://example.invalid/config"])
        with self.assertRaises(oci.Refused):
            oci.blob(self.layout, descriptor)
        path = self.layout / "blobs/sha256" / self.config["digest"].split(":")[1]
        path.unlink()
        path.symlink_to(self.layout / "missing")
        with self.assertRaises(oci.Refused):
            oci.blob(self.layout, self.config)

    def test_publication_is_impossible_from_pr_main_dispatch_or_fork(self):
        good = {"GITHUB_REF": "refs/tags/v0.8.101", "GITHUB_EVENT_NAME": "push", "GITHUB_REPOSITORY": "Ulzuhan/linkup","GITHUB_SHA":SOURCE}
        with patch.dict(os.environ, good, clear=True):
            self.assertEqual(oci.publication_context(SOURCE), "0.8.101")
        for key, value in (("GITHUB_REF", "refs/heads/main"), ("GITHUB_EVENT_NAME", "pull_request"),
                           ("GITHUB_EVENT_NAME", "workflow_dispatch"), ("GITHUB_REPOSITORY", "fork/linkup"),
                           ("GITHUB_REF","refs/tags/v0.8.0"),("GITHUB_REF","refs/tags/v0.8.1-rc.1"),("GITHUB_SHA","a"*40)):
            with patch.dict(os.environ, dict(good, **{key: value}), clear=True):
                with self.assertRaises(oci.Refused):
                    oci.publication_context(SOURCE)

    def test_historical_rollback_label_is_forbidden_even_on_corrected_runtime(self):
        root=copy.deepcopy(self.root)
        config=json.loads(oci.blob(self.layout,self.config));config['config']['Labels']['io.kaicorp.linkup.rollback-image']=oci.ROLLBACK
        manifest=json.loads(oci.blob(self.layout,root['manifests'][0]));manifest['config']=self.put(config)
        descriptor=self.put(manifest);descriptor['platform']={'architecture':'amd64','os':'linux'};root['manifests'][0]=descriptor
        with self.assertRaises(oci.Refused):self.verify(root)

    def test_publication_signature_requires_certificate_source_digest_run_and_attempt(self):
        expected='sha256:'+'c'*64
        good=[{'verificationResult':{'signature':{'certificate':{'runInvocationURI':'https://github.com/Ulzuhan/linkup/actions/runs/123/attempts/2','sourceRepositoryDigest':SOURCE}},'statement':{'subject':[{'name':oci.REPOSITORY,'digest':{'sha256':'c'*64}}]}}}]
        with patch.dict(os.environ,{'GITHUB_RUN_ID':'123','GITHUB_RUN_ATTEMPT':'2'},clear=True):
            with patch.object(oci,'command',return_value=json.dumps(good).encode()) as call:
                oci.verify_publication_signature(expected,SOURCE,'0.8.101')
                self.assertIn('--deny-self-hosted-runners',call.call_args.args)
                self.assertIn('--source-ref',call.call_args.args)
            for fault in ('run','attempt','source','subject','digest','predicate-only'):
                wrong=copy.deepcopy(good);value=wrong[0]['verificationResult'];cert=value['signature']['certificate']
                if fault=='run':cert['runInvocationURI']='https://github.com/Ulzuhan/linkup/actions/runs/124/attempts/2'
                elif fault=='attempt':cert['runInvocationURI']='https://github.com/Ulzuhan/linkup/actions/runs/123/attempts/1'
                elif fault=='source':cert['sourceRepositoryDigest']='a'*40
                elif fault=='subject':value['statement']['subject'][0]['name']='ghcr.io/another/linkup'
                elif fault=='digest':value['statement']['subject'][0]['digest']['sha256']='d'*64
                else:value['statement']['predicate']={'runInvocationURI':cert.pop('runInvocationURI')}
                with self.subTest(fault=fault),patch.object(oci,'command',return_value=json.dumps(wrong).encode()),self.assertRaises(oci.Refused):
                    oci.verify_publication_signature(expected,SOURCE,'0.8.101')

    def test_candidate_and_promote_are_blocked_without_policy_before_registry_access(self):
        for action in ('candidate','promote'):
            args=['oci-release.py',action,'--layout',str(self.layout),'--digest','sha256:'+'c'*64,'--source',SOURCE]
            env={'GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup','GITHUB_SHA':SOURCE}
            # Verification is already gated in this fixture. The real default
            # policy subprocess still refuses, even with a spoofed tag context.
            with patch.object(sys,'argv',args),patch.dict(os.environ,env,clear=True),patch.object(oci,'verify',return_value=self.config['digest']),patch.object(oci,'copy') as copier,patch.object(oci,'immutable_version') as registry:
                with self.assertRaises(oci.Refused):oci.main()
                registry.assert_not_called();copier.assert_not_called()

    def test_prepared_lane_promotes_exact_version_only_without_floating_aliases(self):
        args=['oci-release.py','promote','--layout',str(self.layout),'--digest','sha256:'+'c'*64,'--source',SOURCE]
        env={'GITHUB_REF':'refs/tags/v0.8.101','GITHUB_EVENT_NAME':'push','GITHUB_REPOSITORY':'Ulzuhan/linkup','GITHUB_SHA':SOURCE}
        with patch.object(sys,'argv',args),patch.dict(os.environ,env,clear=True),patch.object(oci,'verify',return_value=self.config['digest']),patch.object(oci,'command',return_value=b''),patch.object(oci,'immutable_version'),patch.object(oci,'verify_publication_signature') as signature,patch.object(oci,'copy') as copier:
            oci.main();signature.assert_called_once_with('sha256:'+'c'*64,SOURCE,'0.8.101')
            copier.assert_called_once_with(self.layout,'sha256:'+'c'*64,'0.8.101')

    def test_release_version_cannot_be_retagged_and_network_error_is_closed(self):
        expected = "sha256:" + hashlib.sha256(b"existing").hexdigest()
        with patch.object(subprocess, "run", return_value=subprocess.CompletedProcess([], 0, b"existing", b"")):
            oci.immutable_version("0.8.0", expected)
            with self.assertRaises(oci.Refused):
                oci.immutable_version("0.8.0", "sha256:" + "a" * 64)
        with patch.object(subprocess, "run", return_value=subprocess.CompletedProcess([], 1, b"", b"network failed")):
            with self.assertRaises(oci.Refused):
                oci.immutable_version("0.8.0", expected)
        with patch.object(subprocess, "run", return_value=subprocess.CompletedProcess([], 1, b"", b"manifest unknown")):
            oci.immutable_version("0.8.0", expected)

    def test_copy_uses_all_preserves_digest_and_rejects_changed_copy(self):
        authdir = self.layout / "auth"
        authdir.mkdir()
        (authdir / "config.json").write_text("{}")
        expected = "sha256:" + "c" * 64
        def copier(*args):
            self.assertIn("--all", args)
            self.assertIn("--preserve-digests", args)
            self.assertNotIn("--dest-creds", args)
            self.assertEqual(args[-2:], ("oci:" + str(self.layout), "docker://ghcr.io/ulzuhan/linkup:0.8.0"))
            Path(args[args.index("--digestfile") + 1]).write_text(expected)
            return b""
        with patch.dict(os.environ, {"DOCKER_CONFIG": str(authdir)}), patch.object(oci, "command", side_effect=copier):
            oci.copy(self.layout, expected, "0.8.0")
        def changed(*args):
            Path(args[args.index("--digestfile") + 1]).write_text("sha256:" + "a" * 64)
            return b""
        with patch.dict(os.environ, {"DOCKER_CONFIG": str(authdir)}), patch.object(oci, "command", side_effect=changed):
            with self.assertRaises(oci.Refused):
                oci.copy(self.layout, expected, "0.8.0")


if __name__ == "__main__":
    unittest.main()
