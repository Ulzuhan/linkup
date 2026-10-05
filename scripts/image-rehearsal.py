#!/usr/bin/env python3
"""Synthetic exact-image return and SQLite backup; never accepts a live data path."""
import argparse
import base64
import contextlib
import hashlib
import http.server
import json
import os
from pathlib import Path
import re
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[1]
SYNTHETIC_SECRET = "synthetic-only-" * 3


def command(*args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=180)
    if result.returncode:
        raise RuntimeError("synthetic command failed: " + args[0])
    return result.stdout.strip()


def inventory(root):
    allowed = {"linkup.db", "linkup.db-wal", "linkup.db-shm"}
    if any(p.is_symlink() or not p.is_file() or p.name not in allowed for p in root.iterdir()):
        raise ValueError("unexpected associated file: manual backup/format review")


def integrity(db):
    assert db.execute("PRAGMA integrity_check").fetchall() == [("ok",)]
    assert db.execute("PRAGMA foreign_key_check").fetchall() == []


def schema(db):
    def normalized(conn):
        return sorted((kind, name, " ".join(sql.split())) for kind, name, sql in
                      conn.execute("SELECT type,name,sql FROM sqlite_master WHERE sql IS NOT NULL"))
    expected = sqlite3.connect(":memory:")
    for sql in re.findall(r"`(CREATE.*?)`", (ROOT / "internal/database/schema.go").read_text(), re.S):
        expected.execute(sql)
    if normalized(db) != normalized(expected):
        raise ValueError("foreign, partial or changed schema: manual migration review")
    expected.close()


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args):
        return None


def request(base, path, method="GET", data=None, headers=None):
    payload = json.dumps(data).encode() if data is not None else None
    req = urllib.request.Request(base + path, data=payload, method=method,
                                 headers={"Content-Type": "application/json", "Host": "link.example.invalid", **(headers or {})})
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    try:
        response = opener.open(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.code, response.read(), response.headers


@contextlib.contextmanager
def serve(image, data, oidc=None):
    assert data.parent.name.startswith("linkup-image-") and data.name == "data"
    assert image.startswith("sha256:") or image.startswith("ghcr.io/ulzuhan/linkup@sha256:")
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
    name = "linkup-image-" + uuid.uuid4().hex
    args = ["docker", "run", "--detach", "--name", name, "--network", "host", "--init",
            "--stop-timeout", "10", "--read-only", "--cap-drop", "ALL",
            "--security-opt", "no-new-privileges:true", "--user", "10001:10001",
            "--pids-limit", "128", "--memory", "128m", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=32m",
            "--mount", "type=bind,source=" + str(data) + ",target=/data",
            "--health-cmd", f"wget -q --spider http://127.0.0.1:{port}/healthz",
            "-e", "LINKUP_HOST=127.0.0.1", "-e", "LINKUP_PORT=" + str(port),
            "-e", "LINKUP_DB_PATH=/data/linkup.db", "-e", "LINKUP_PUBLIC_HOST=link.example.invalid",
            "-e", "LINKUP_DEFAULT_DOMAIN=link.example.invalid", "-e", "LINKUP_SESSION_SECRET=" + SYNTHETIC_SECRET]
    if oidc:
        args += ["-e", "LINKUP_OIDC_ISSUER=" + oidc["issuer"], "-e", "LINKUP_OIDC_INTERNAL_BASE=" + oidc["internal"],
                 "-e", "LINKUP_OIDC_CLIENT_ID=synthetic", "-e", "LINKUP_OIDC_CLIENT_SECRET=synthetic",
                 "-e", "LINKUP_OIDC_REDIRECT_URI=https://link.example.invalid/auth/callback"]
    else:
        # Loopback-only dev identity belongs solely to this disposable fixture.
        args += ["-e", "LINKUP_DEV_MODE=true"]
    cid = command(*args, image)
    assert re.fullmatch(r"[a-f0-9]{64}", cid)
    base = "http://127.0.0.1:" + str(port)
    try:
        deadline = time.monotonic() + 30
        while True:
            try:
                status, body, _ = request(base, "/healthz")
                assert status == 200 and json.loads(body) == {"status": "healthy", "service": "linkup"}
                break
            except (OSError, AssertionError):
                if time.monotonic() >= deadline:
                    raise RuntimeError("synthetic runtime health timeout")
                time.sleep(0.2)
        yield base
    finally:
        command("docker", "stop", "--time", "10", cid)
        command("docker", "rm", cid)


def create(base, slug, url="https://example.org/first", **fields):
    status, body, _ = request(base, "/api/links", "POST", dict(url=url, custom_slug=slug, **fields))
    assert status == 201
    value = json.loads(body)["link"]
    assert value["slug"] == slug and value["created_by"] == "dev-user-id"
    assert 1_000_000_000 < value["created_at"] < 10_000_000_000
    return value["id"]


def update(base, identity, **fields):
    assert request(base, "/api/links/" + identity, "PUT", fields)[0] == 200


def credentials(now):
    from cryptography.hazmat.primitives.ciphers.aead import AESGCM
    key = hashlib.sha256(SYNTHETIC_SECRET.encode()).digest()
    def encrypt(raw):
        nonce = os.urandom(12)
        return base64.urlsafe_b64encode(nonce + AESGCM(key).encrypt(nonce, raw, None)).decode()
    cookie = encrypt(json.dumps({"user_id": "dev-user-id", "username": "dev-user", "email": "synthetic@example.invalid",
                                 "session_id": "revoked", "is_admin": False, "created_at": now}).encode())
    api_key = "lk_live_" + os.urandom(24).hex()
    return {"cookie": cookie, "access_token": encrypt(b"synthetic-access"), "api_key": api_key,
            "key_hash": hashlib.sha256(api_key.encode()).hexdigest()}


def oidc_smoke(image, data, auth, expected):
    issuer = "https://id.example.invalid/auth/v1"
    class Provider(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass
        def do_GET(self):
            internal = "http://127.0.0.1:" + str(self.server.server_port)
            if self.path == "/auth/v1/userinfo":
                self.send_response(200 if self.headers.get("Authorization") == "Bearer synthetic-access" else 401)
                self.send_header("Content-Type", "application/json");self.end_headers()
                self.wfile.write(json.dumps({"sub": "dev-user-id", "preferred_username": "dev-user",
                                            "email": "synthetic@example.invalid", "groups": []}).encode())
                return
            doc = {"issuer": issuer, "authorization_endpoint": internal + "/auth/v1/authorize",
                   "token_endpoint": internal + "/auth/v1/token", "jwks_uri": internal + "/auth/v1/jwks",
                   "userinfo_endpoint": internal + "/auth/v1/userinfo",
                   "response_types_supported": ["code"], "subject_types_supported": ["public"],
                   "id_token_signing_alg_values_supported": ["RS256"]}
            self.send_response(200);self.send_header("Content-Type", "application/json");self.end_headers()
            self.wfile.write(json.dumps(doc if self.path.endswith("openid-configuration") else {"keys": []}).encode())
    provider = http.server.HTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=provider.serve_forever, daemon=True);thread.start()
    try:
        with serve(image, data, {"issuer": issuer, "internal": "http://127.0.0.1:" + str(provider.server_port)}) as base:
            assert request(base, "/api/links")[0] == 401
            status, _, headers = request(base, "/auth/login")
            target = urllib.parse.urlsplit(headers["Location"]);query = urllib.parse.parse_qs(target.query)
            assert status == 303 and target.netloc == "id.example.invalid" and target.path == "/auth/v1/authorize"
            assert query["code_challenge_method"] == ["S256"] and query["response_type"] == ["code"]
            assert len(query["state"]) == len(query["code_challenge"]) == 1 and query["state"][0] and query["code_challenge"][0]
            assert query["redirect_uri"] == ["https://link.example.invalid/auth/callback"]
            assert request(base, "/api/links", headers={"Cookie": "linkup_session=" + auth["cookie"]})[0] == expected
            assert request(base, "/api/links", headers={"Authorization": "Bearer " + auth["api_key"]})[0] == expected
    finally:
        provider.shutdown();provider.server_close();thread.join(timeout=2)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--candidate", required=True)
    parser.add_argument("--baseline", required=True)
    args = parser.parse_args()
    command("docker", "pull", args.baseline)
    with tempfile.TemporaryDirectory(prefix="linkup-image-") as tmp:
        root = Path(tmp);root.chmod(0o755);data = root / "data";data.mkdir(mode=0o777);data.chmod(0o777)
        with serve(args.baseline, data) as base:
            ids = {name: create(base, name) for name in ("printed", "paused", "deleted", "budget")}
            assert request(base, "/printed")[0] == 302
            deadline = time.monotonic() + 5
            db = sqlite3.connect(data / "linkup.db")
            while db.execute("SELECT click_count FROM links WHERE id=?", (ids["printed"],)).fetchone()[0] < 1:
                assert time.monotonic() < deadline;time.sleep(0.05)
            now = int(time.time())
            auth = credentials(now)
            db.execute("INSERT INTO oidc_sessions VALUES(?,?,?,?,?,?)", ("revoked", "dev-user-id", "sid", "dev-user", auth["access_token"], now + 3600))
            db.execute("INSERT INTO api_keys VALUES(?,?,?,?,?,?,?)", ("deleted-key", "dev-user-id", "key", "prefix", auth["key_hash"], None, now))
            db.execute("INSERT INTO webhooks VALUES(?,?,?,?,?,?,?)", ("webhook", "sub", "https://example.org/hook", "synthetic", "link.created", 0, now))
            db.commit();integrity(db);schema(db);inventory(data)
            source = sqlite3.connect("file:" + str(data / "linkup.db") + "?mode=ro", uri=True)
            backup = sqlite3.connect(root / "backup.db");source.backup(backup);integrity(backup);schema(backup)
            # Includes committed WAL while the source writer remains running.
            assert backup.execute("SELECT click_count FROM links WHERE id=?", (ids["printed"],)).fetchone() == (1,)
            backup.close();source.close();db.close()
        oidc_smoke(args.baseline, data, auth, 200)
        with serve(args.candidate, data) as base:
            assert request(base, "/printed")[0] == 302
            update(base, ids["printed"], target_url="https://example.org/after")
            update(base, ids["paused"], is_active=False)
            update(base, ids["budget"], max_clicks=1)
            assert request(base, "/api/links/" + ids["deleted"], "DELETE")[0] == 200
            ids["new"] = create(base, "new-after-backup", "https://example.org/new")
            assert request(base, "/paused")[0] == 410 and request(base, "/deleted")[0] == 404
            db = sqlite3.connect(data / "linkup.db");db.execute("DELETE FROM oidc_sessions WHERE id='revoked'")
            db.execute("DELETE FROM api_keys WHERE id='deleted-key'");db.execute("INSERT INTO oidc_logout_jtis VALUES(?,?)", ("logout", now + 3600));db.commit()
            integrity(db);schema(db);db.close()
        with serve(args.baseline, data) as base:
            assert request(base, "/printed")[2]["Location"] == "https://example.org/after"
            assert request(base, "/new-after-backup")[0] == 302
            assert request(base, "/paused")[0] == 410 and request(base, "/deleted")[0] == 404
            db = sqlite3.connect(data / "linkup.db");integrity(db);schema(db);inventory(data)
            assert db.execute("SELECT count(*) FROM oidc_sessions WHERE id='revoked'").fetchone() == (0,)
            assert db.execute("SELECT count(*) FROM api_keys WHERE id='deleted-key'").fetchone() == (0,)
            assert db.execute("SELECT count(*) FROM oidc_logout_jtis WHERE jti='logout'").fetchone() == (1,)
            assert db.execute("SELECT secret FROM webhooks WHERE id='webhook'").fetchone() == ("synthetic",)
            assert db.execute("SELECT created_by FROM links WHERE id=?", (ids["printed"],)).fetchone() == ("dev-user-id",)
            assert db.execute("SELECT max_clicks FROM links WHERE id=?", (ids["budget"],)).fetchone() == (1,)
            db.close()
        oidc_smoke(args.candidate, data, auth, 401)
        oidc_smoke(args.baseline, data, auth, 401)
        # Negative control: restore into a NEW target, never the current writer's path.
        old = sqlite3.connect("file:" + str(root / "backup.db") + "?mode=ro", uri=True)
        restored = sqlite3.connect(root / "separate-restored.db");old.backup(restored);integrity(restored);schema(restored)
        assert restored.execute("SELECT count(*) FROM links WHERE id=?", (ids["new"],)).fetchone() == (0,)
        assert restored.execute("SELECT target_url FROM links WHERE id=?", (ids["printed"],)).fetchone() == ("https://example.org/first",)
        assert restored.execute("SELECT count(*) FROM oidc_sessions WHERE id='revoked'").fetchone() == (1,)
        assert restored.execute("SELECT count(*) FROM api_keys WHERE id='deleted-key'").fetchone() == (1,)
        restored.close();old.close()
        with tempfile.TemporaryDirectory(prefix="linkup-image-") as restore_tmp:
            target_root = Path(restore_tmp);target_root.chmod(0o755)
            target = target_root / "data";target.mkdir(mode=0o777);target.chmod(0o777)
            old = sqlite3.connect("file:" + str(root / "backup.db") + "?mode=ro", uri=True)
            restored = sqlite3.connect(target / "linkup.db");old.backup(restored);restored.close();old.close()
            (target / "linkup.db").chmod(0o666)
            oidc_smoke(args.baseline, target, auth, 200)
        print("PASS: exact baseline -> candidate -> same baseline over current SQLite, one app writer at a time")
        print("PASS: API writes, slugs/owners/seconds, pause/delete, budgets, logout replay IDs and key/session deletions preserved")
        print("PASS: WAL-consistent backup and separate restore integrity/schema; OIDC discovery/login/PKCE smoke")
        print("PROVEN LIMIT: stale restore loses new writes and revives deleted session/key; manual data restore only")


if __name__ == "__main__":
    main()
