#!/usr/bin/env python3
"""End-to-end checks for a disposable Compose demo (changes worker fault injection)."""
import json
import os
import subprocess
import time
import uuid


def compose(*args, env=None, check=True):
    result = subprocess.run(["docker", "compose", *args], text=True,
                            capture_output=True, env=env, timeout=180)
    if check and result.returncode:
        raise RuntimeError(result.stderr + result.stdout)
    return result


def cli(*args, ok=True):
    result = compose("run", "--rm", "--no-deps", "-T", "cli", *args, check=False)
    if ok and result.returncode:
        raise RuntimeError(result.stderr + result.stdout)
    if not ok:
        assert result.returncode != 0, result.stdout
    # Application diagnostics must remain on stderr.
    return json.loads(result.stdout)


def worker(failure=""):
    env = dict(os.environ, FAIL_AT=failure)
    compose("up", "-d", "--no-deps", "worker", env=env)


def wait_operation(oid, status):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        op = cli("operation", "-id", oid)
        if op["status"] == status:
            return op
        time.sleep(0.5)
    raise AssertionError(op)


def main():
    prefix = "smoke-" + uuid.uuid4().hex[:8]
    compose("up", "-d", "--build")
    compose("build", "cli")
    worker()
    vmid = prefix + "-life"
    cli("create", "-id", vmid, "-cpu", "2", "-memory", "2048", "-wait")
    initial = cli("get", "-id", vmid)
    assert initial["vm"]["state"] == "running" and initial["compute"]
    cli("stop", "-id", vmid, "-wait")
    stopped = cli("get", "-id", vmid)
    assert stopped["vm"]["state"] == "stopped" and stopped["compute"] is None
    assert initial["disk"] == stopped["disk"] and initial["ip"] == stopped["ip"]
    cli("start", "-id", vmid, "-wait")
    # Симулятор продолжает работать при перезапуске остальных процессов.
    simulator_id = compose("ps", "-q", "simulator").stdout.strip()
    assert simulator_id
    compose("stop", "worker", "outbox", "reconciler", "temporal", "postgres")
    compose("up", "-d")
    assert compose("ps", "-q", "simulator").stdout.strip() == simulator_id
    assert cli("get", "-id", vmid)["vm"]["state"] == "running"
    compose("stop", "worker")
    pending = cli("stop", "-id", vmid)
    assert pending["status"] == "pending"
    # Outbox запускает workflow даже без исполнителя.
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        delivered = cli("operation", "-id", pending["id"])
        if delivered["run_id"]:
            break
        time.sleep(0.5)
    assert delivered["run_id"] and delivered["status"] == "pending", delivered
    worker()
    wait_operation(pending["id"], "succeeded")
    # Без outbox команда остаётся в БД, а после запуска доставляется.
    compose("stop", "outbox")
    queued = cli("start", "-id", vmid)
    time.sleep(2)
    assert cli("operation", "-id", queued["id"])["run_id"] == ""
    compose("up", "-d", "--no-deps", "outbox")
    wait_operation(queued["id"], "succeeded")
    # Два worker не создают дополнительных фоновых циклов.
    compose("stop", "reconciler")
    compose("up", "-d", "--no-deps", "--scale", "worker=2", "worker")
    services = [
        json.loads(line) for line in compose("ps", "--format", "json").stdout.splitlines()]
    assert sum(service["Service"] == "worker" for service in services) == 2
    assert sum(service["Service"] == "outbox" for service in services) == 1
    assert sum(service["Service"] == "simulator" for service in services) == 1
    compose("restart", "worker")
    assert compose("ps", "-q", "simulator").stdout.strip() == simulator_id
    cli("stop", "-id", vmid, "-wait")
    cli("start", "-id", vmid, "-wait")
    compose("up", "-d", "--no-deps", "--scale", "worker=1", "worker")
    compose("up", "-d", "--no-deps", "reconciler")
    cli("delete", "-id", vmid, "-wait")
    deleted = cli("get", "-id", vmid)
    assert deleted["vm"]["state"] == "deleted"
    assert all(deleted[k] is None for k in ("compute", "disk", "ip"))
    full = cli("create", "-id", prefix + "-full", "-cpu", "999", "-wait", ok=False)
    assert full["status"] == "failed"
    assert cli("get", "-id", prefix + "-full")["compute"] is None
    worker("wait-ready")
    failed = cli("create", "-id", prefix + "-failure", "-wait", ok=False)
    assert failed["status"] == "failed"
    worker()
    cli("create", "-id", prefix + "-failure", "-wait")
    worker("stop")
    blocked = cli("delete", "-id", prefix + "-failure", "-wait", ok=False)
    assert blocked["status"] == "rollback-blocked"
    assert cli("get", "-id", prefix + "-failure")["compute"] is not None
    worker()
    cli("cleanup", "-id", prefix + "-failure", "-wait")
    assert cli("get", "-id", prefix + "-failure")["vm"]["state"] == "deleted"
    worker("retry:wait-ready")
    terminated = cli("create", "-id", prefix + "-terminate")
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        snapshot = cli("get", "-id", prefix + "-terminate")
        if snapshot["vm"]["state"] == "running":
            break
        time.sleep(0.1)
    assert snapshot["vm"]["state"] == "running"
    compose("exec", "-T", "temporal", "temporal", "workflow", "terminate",
            "--workflow-id", terminated["workflow_id"], "--reason", "smoke test",
            "--address", "temporal:7233")
    wait_operation(terminated["id"], "rollback-blocked")
    snapshot = cli("get", "-id", prefix + "-terminate")
    assert snapshot["compute"] and snapshot["vm"]["recovery_required"]
    worker()
    cli("cleanup", "-id", prefix + "-terminate", "-wait")
    snapshot = cli("get", "-id", prefix + "-terminate")
    assert snapshot["vm"]["state"] == "not-created" and snapshot["compute"] is None
    print("Compose smoke passed:", prefix)


if __name__ == "__main__":
    try:
        main()
    finally:
        worker()
