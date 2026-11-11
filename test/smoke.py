"""Exercise a built Termihomo binary and the official Mihomo release on Linux."""

import fcntl
import http.server
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import threading
import time
import urllib.error
import urllib.request


def profile(group):
    return f"""allow-lan: true
external-controller: 0.0.0.0:9090
profile:
  store-selected: false
proxy-groups:
  - name: {group}
    type: select
    proxies: [REJECT, DIRECT]
rules:
  - MATCH,{group}
""".encode()


class LocalServer(http.server.BaseHTTPRequestHandler):
    subscription = profile("Pick")

    def do_HEAD(self):
        if self.path == "/delay":
            time.sleep(2.2)
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        self.do_HEAD()
        body = self.subscription if self.path.startswith("/sub?") else b"proxy-ok"
        self.wfile.write(body)

    def log_message(self, *_args):
        pass


def smoke(binary, directory, base):
    env = dict(
        os.environ,
        TERM="xterm-256color",
        TERMIHOMO_LANG="en",
        XDG_CONFIG_HOME=str(directory / "config"),
        XDG_DATA_HOME=str(directory / "data"),
        XDG_STATE_HOME=str(directory / "state"),
        CLASH_OVERRIDE_EXTERNAL_CONTROLLER="0.0.0.0:9090",
    )

    def command(*args, ok=True, input=None):
        result = subprocess.run(
            [binary, *args], input=input, stdin=subprocess.DEVNULL if input is None else None,
            capture_output=True, text=True, env=env, timeout=160, check=False,
        )
        if ok and result.returncode:
            raise AssertionError(f"{args[:2]} exited {result.returncode}: {result.stderr}")
        return result

    def status():
        return json.loads(command("status", "--json").stdout)

    def request(state):
        output = subprocess.check_output(
            ["curl", "-fsS", "--noproxy", "", "-x", f"http://{state['proxy']}", f"{base}/echo"],
            timeout=10,
        )
        assert output == b"proxy-ok"

    assert status()["state"] == "NotInstalled"
    command("install")
    assert status()["state"] == "Stopped"
    token = "termihomo-smoke-secret"
    command("profile", "add", "-", input=f"{base}/sub?token={token}")
    command("config", "check")
    try:
        command("start")
        running = status()
        assert running["state"] == "Running", running
        assert json.loads(command("--lang", "zh-CN", "status", "--json").stdout) == running
        assert "运行中" in command("--lang", "zh-CN", "status").stdout
        assert running["controller"].startswith("127.0.0.1:")
        try:
            urllib.request.build_opener(urllib.request.ProxyHandler({})).open(
                f"http://{running['controller']}/version", timeout=5
            )
            raise AssertionError("controller accepted an unauthenticated request")
        except urllib.error.HTTPError as error:
            assert error.code == 401
        assert command("stop", ok=False).returncode == 2
        terminal_smoke(binary, env, running, status, request, command)

        LocalServer.subscription = b"broken: [\n"
        invalid = command("profile", "update", "--yes", ok=False)
        assert invalid.returncode == 1 and token not in invalid.stderr
        request(running)

        running = reject_reload(directory, command, status, request, running)

        if os.geteuid() != 0:
            # Prevent only the final profile record write. The managed
            # runtime file remains writable and the candidate reload succeeds.
            config_dir = directory / "config" / "termihomo"
            config_dir.chmod(0o500)
            try:
                LocalServer.subscription = profile("Candidate")
                rollback = command("profile", "update", "--yes", ok=False)
            finally:
                config_dir.chmod(0o700)
            assert rollback.returncode == 1 and "restored previous configuration" in rollback.stderr, rollback.stderr
            restored = status()
            assert restored["state"] == "Running" and restored["proxy"] == running["proxy"], restored
            request(restored)
            running = restored

        LocalServer.subscription = profile("Next")
        command("profile", "update", "--yes")
        assert '"Next"' in command("proxy", "list").stdout
        command("proxy", "select", "Next", "DIRECT")
        request(running)
        assert " ms" in command("proxy", "test", "DIRECT", f"{base}/delay").stdout
        assert Path(command("config", "backup").stdout.strip()).stat().st_mode & 0o777 == 0o600
        assert token not in command("logs").stdout

        command("restart", "--yes")
        restarted = status()
        assert restarted["state"] == "Running" and restarted["pid"] != running["pid"], restarted
        os.kill(restarted["pid"], signal.SIGSTOP)
        try:
            assert status()["state"] == "Degraded"
        finally:
            os.kill(restarted["pid"], signal.SIGCONT)
        os.kill(restarted["pid"], signal.SIGKILL)
        deadline = time.monotonic() + 5
        while status()["state"] != "Failed":
            assert time.monotonic() < deadline, "abnormal exit not detected"
            time.sleep(0.05)
    finally:
        command("stop", "--yes")
    assert status()["state"] == "Stopped"


def reject_reload(directory, command, status, request, running):
    class Controller(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            upstream = urllib.request.Request(
                f"http://{running['controller']}{self.path}",
                headers={"Authorization": self.headers["Authorization"]},
            )
            opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
            with opener.open(upstream, timeout=2) as response:
                self.send_response(response.status)
                self.end_headers()
                self.wfile.write(response.read())

        def do_PUT(self):
            assert self.path == "/configs"
            self.send_response(503)
            self.end_headers()

        def log_message(self, *_args):
            pass

    # Inject a controller failure at the HTTP boundary while forwarding the
    # status and selector reads to the real kernel.
    instance_path = directory / "data" / "termihomo" / "instance.json"
    record = json.loads(instance_path.read_text())
    with http.server.ThreadingHTTPServer(("127.0.0.1", 0), Controller) as server:
        threading.Thread(target=server.serve_forever, daemon=True).start()
        record["Controller"] = f"127.0.0.1:{server.server_port}"
        instance_path.write_text(json.dumps(record))
        LocalServer.subscription = profile("Rejected")
        try:
            failure = command("profile", "update", "--yes", ok=False)
        finally:
            server.shutdown()
    assert failure.returncode == 1 and "restored previous configuration" in failure.stderr, failure.stderr
    assert "HTTP 503" in failure.stderr, failure.stderr
    restored = status()
    assert restored["state"] == "Running" and restored["pid"] != running["pid"], restored
    assert restored["proxy"] == running["proxy"], restored
    assert '"Pick"' in command("proxy", "list").stdout
    request(restored)
    return restored


def terminal_smoke(binary, env, running, status, request, command):
    master, slave = pty.openpty()
    original = termios.tcgetattr(slave)
    process = None
    seen = bytearray()

    def resize(cols, rows):
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))

    def launch(*args):
        seen.clear()
        return subprocess.Popen(
            [binary, *args], stdin=slave, stdout=slave, stderr=slave, env=env, start_new_session=True,
        )

    def expect(text):
        deadline = time.monotonic() + 12
        needle = re.compile(re.escape(text.encode()) if isinstance(text, str) else text)
        match = needle.search(seen)
        while match is None:
            if time.monotonic() >= deadline:
                raise AssertionError(f"terminal did not show {text!r}: {seen[-500:]!r}")
            if select.select([master], [], [], 0.15)[0]:
                seen.extend(os.read(master, 65536))
                match = needle.search(seen)
        del seen[:match.end()]

    def key(data):
        os.write(master, data)

    try:
        resize(100, 27)
        process = launch()
        expect("Running")
        expect("total")
        assert status()["pid"] == running["pid"]
        key(b"2")
        expect("DIRECT")
        key(b"3")
        expect("Pick")
        key(b"j\r")  # Pick follows the built-in GLOBAL group.
        expect("Enter select")
        key(b"j\r")  # DIRECT follows REJECT in the fixture.
        deadline = time.monotonic() + 12
        while '"Pick"\tSelector\t"DIRECT"' not in command("proxy", "list").stdout:
            assert time.monotonic() < deadline, "terminal did not select DIRECT"
            time.sleep(0.1)
        key(b"6")
        expect("Mihomo logs")
        request(running)
        expect("Pick[DIRECT]")
        key(b"5")
        expect(rb"total .*?(?:[1-9][0-9]*\.[0-9]|0\.[1-9]) (?:B|KiB|MiB)")
        resize(37, 10)
        process.send_signal(signal.SIGWINCH)
        key(b"1")
        expect("Overview")
        key(b"q")
        assert process.wait(timeout=5) == 0
        assert termios.tcgetattr(slave) == original
        assert status()["pid"] == running["pid"]
        request(running)
        process = launch("--lang", "zh-CN")
        expect("总览")
        expect("运行中")
        key(b"4")
        expect("添加订阅")
        key(b"a")
        expect("订阅地址（已隐藏")
        key(b"\x1b")
        expect("退出")
        key(b"q")
        assert process.wait(timeout=5) == 0
    finally:
        if process is not None and process.poll() is None:
            process.kill()
            process.wait()
        os.close(slave)
        os.close(master)


if __name__ == "__main__":
    with tempfile.TemporaryDirectory(prefix="termihomo-smoke-") as tmp:
        with http.server.ThreadingHTTPServer(("127.0.0.1", 0), LocalServer) as server:
            threading.Thread(target=server.serve_forever, daemon=True).start()
            try:
                smoke(Path(sys.argv[1]).resolve(), Path(tmp), f"http://127.0.0.1:{server.server_port}")
            finally:
                server.shutdown()
    print("Termihomo runtime acceptance passed")
