"""Exercise packaging in an isolated disposable Compose project (Docker + Python 3)."""
import io
import json
import os
from pathlib import Path
import secrets
import subprocess
import tarfile
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
os.chdir(ROOT)
env = dict(os.environ, COMPOSE_PROJECT_NAME="gowork-m9-" + secrets.token_hex(6),
           JWT_SECRET=secrets.token_hex(32))
compose = ["docker", "compose", "-f", str(ROOT / "docker-compose.yml")]


def run(args, *, check=True, capture=False):
    return subprocess.run(args, env=env, check=check, text=True,
                          stdout=subprocess.PIPE if capture else None)


def dc(*args, **kwargs):
    return run(compose + list(args), **kwargs)


def sql(query):
    return dc("exec", "-T", "postgres", "psql", "-U", "gowork_owner", "-d", "gowork",
              "-At", "-v", "ON_ERROR_STOP=1", "-c", query, capture=True).stdout.strip()


def request(path, data=None, token=None):
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(
        "http://127.0.0.1:" + env.get("API_PORT", "8080") + path,
        data=json.dumps(data).encode() if data is not None else None, headers=headers)
    with urllib.request.urlopen(req, timeout=10) as response:
        return json.load(response)


try:
    dc("config", "--quiet")
    dc("build")
    dc("up", "-d", "--wait", "--wait-timeout", "120")
    api = dc("ps", "-q", "api", capture=True).stdout.strip()
    config = json.loads(run(["docker", "inspect", api], capture=True).stdout)[0]
    assert config["Config"]["User"] == "65532:65532"
    assert config["Config"]["Entrypoint"] == ["/app/gowork-api"]
    assert config["HostConfig"]["ReadonlyRootfs"]
    assert not any(e.startswith(("MIGRATION_DATABASE_URL=", "PGPASSWORD="))
                   for e in config["Config"]["Env"])
    image = json.loads(run(["docker", "image", "inspect", config["Image"]], capture=True).stdout)[0]
    assert not any(e.startswith(("JWT_SECRET=", "DATABASE_URL=", "MIGRATION_DATABASE_URL="))
                   for e in image["Config"]["Env"])
    # Inspect the actual final filesystem, including CA roots and absence of build inputs.
    exported = subprocess.check_output(["docker", "export", api], env=env)
    with tarfile.open(fileobj=io.BytesIO(exported)) as archive:
        names = set(archive.getnames())
        assert "etc/ssl/certs/ca-certificates.crt" in names
        assert archive.getmember("etc/ssl/certs/ca-certificates.crt").size > 10000
        assert any(
            n.startswith("app/db/migrations/") and n.endswith(".up.sql") for n in names)
        assert not any(n.endswith((".go", ".env", "/go.mod")) for n in names)
        assert "bin/sh" not in names
    print("Runtime image bytes:", image["Size"], flush=True)
    request("/health")
    request("/ready")
    assert sql("SELECT count(*) FROM users") == "0", "Startup must not seed demo accounts"
    dc("run", "--rm", "--no-deps", "--entrypoint", "/app/gowork-seed", "api")
    token = request("/api/v1/auth/login", {
        "email": "admin@gowork.dev", "password": "GoWork-demo-only-2026!"})["data"]["access_token"]
    asset = request("/api/v1/assets", {
        "asset_code": "M9-PERSIST", "name": "Docker persistence", "category": "IT", "location": "CI",
        "status": "ACTIVE"}, token)["data"]
    before = sql("SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM assets), "
                 "(SELECT count(*) FROM audit_logs)")
    dc("run", "--rm", "--no-deps", "--entrypoint", "/app/gowork-seed", "api")
    assert sql("SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM assets), "
               "(SELECT count(*) FROM audit_logs)") == before
    # Down keeps the named volume; up must rerun migration/grants without resetting data.
    dc("down")
    dc("up", "-d", "--wait", "--wait-timeout", "120")
    assert request("/api/v1/assets/" + asset["id"], token=token)["data"] == asset
    assert sql("SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM assets), "
               "(SELECT count(*) FROM audit_logs)") == before
    # A database outage affects readiness, but not process liveness.
    dc("stop", "postgres")
    dc("exec", "-T", "api", "/app/gowork-probe", "/health")
    assert dc("exec", "-T", "api", "/app/gowork-probe", "/ready", check=False).returncode != 0
    dc("start", "--wait", "--wait-timeout", "120", "postgres")
    dc("up", "-d", "--wait", "--wait-timeout", "120", "api")
    api = dc("ps", "-q", "api", capture=True).stdout.strip()
    dc("stop", "api")
    state = json.loads(run(["docker", "inspect", api], capture=True).stdout)[0]["State"]
    assert state["ExitCode"] == 0, state
    # Failed one-shot migration must gate the API, even when the schema already exists.
    dc("down")
    with tempfile.TemporaryDirectory() as temporary:
        override = Path(temporary) / "failure.json"
        override.write_text(json.dumps({"services": {"migrate": {"command": ["invalid"]}}}))
        result = run(compose + ["-f", str(override), "up", "-d", "api"], check=False)
        assert result.returncode != 0, "Failed migration must fail startup"
        assert dc("ps", "--status", "running", "-q", "api", capture=True).stdout.strip() == ""
    print("Docker packaging smoke passed: fresh startup, least privilege image, HTTP, "
          "persistent volume, idempotent seed, outage probes, SIGTERM, migration gate.", flush=True)
finally:
    # Generated unique project only; never removes the developer's usual Compose volume.
    dc("down", "--volumes", "--remove-orphans", check=False)
