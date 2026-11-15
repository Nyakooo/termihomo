"""Exercise a built Termihomo binary and the official Mihomo release on the host operating system."""

import http.server
import json
import ipaddress
import socket
import os
from pathlib import Path
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.request

if os.name != "nt":
    import fcntl
    import pty
    import termios


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
        if self.path.startswith("/sub?"):
            self.send_header("Subscription-Userinfo", "upload=1024; download=2048; total=10240; expire=2051222400")
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
        NO_COLOR="1",
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
    command("init")
    with socket.socket() as reservation:
        reservation.bind(("127.0.0.1", 0))
        port = reservation.getsockname()[1]
    overrides = directory / "config" / "termihomo" / "overrides.yaml"
    overrides.write_text(f"mixed-port: {port}\n")
    command("install")
    assert status()["state"] == "Stopped"
    token = "termihomo-smoke-secret"
    command("profile", "add", "-", input=f"{base}/sub?token={token}")
    command("config", "check")
    try:
        paused_pid = cold_proxy_controller(binary, env, status) if os.name != "nt" else None
        command("start")
        running = status()
        assert paused_pid is None or running["pid"] == paused_pid, running
        assert running["proxy"] == f"127.0.0.1:{port}", running
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
        if os.name != "nt":
            running = terminal_smoke(binary, env, running, status, request, command, base)
        else:
            command("proxy", "select", "Pick", "DIRECT")
            request(running)
        verify_service_probe(directory, base, running, request)

        LocalServer.subscription = b"broken: [\n"
        invalid = command("profile", "update", "--yes", ok=False)
        assert invalid.returncode == 1 and token not in invalid.stderr
        request(running)

        running = reject_reload(directory, command, status, request, running)

        if os.name != "nt" and os.geteuid() != 0:
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
        backup = Path(command("config", "backup").stdout.strip()).stat()
        if os.name != "nt":
            assert backup.st_mode & 0o777 == 0o600
        assert token not in command("logs").stdout

        command("restart", "--yes")
        restarted = status()
        assert restarted["state"] == "Running" and restarted["pid"] != running["pid"], restarted
        assert '"Next"\tSelector\t"DIRECT"' in command("proxy", "list").stdout
        request(restarted)
        if os.name == "nt":
            subprocess.run(["taskkill", "/F", "/PID", str(restarted["pid"])], check=True)
        else:
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


def verify_service_probe(directory, base, running, request):
    instance = json.loads((directory / "data" / "termihomo" / "instance.json").read_text())
    secret = instance["Secret"]
    proxy = instance["ServiceProbe"]
    assert proxy.startswith("127.0.0.1:") and proxy != running["proxy"]
    controller = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    for node in ("REJECT", "DIRECT"):
        address = f"http://{running['controller']}/proxies/__termihomo_service_probe"
        call = urllib.request.Request(address, json.dumps({"name": node}).encode(), method="PUT",
                                      headers={"Authorization": "Bearer " + secret, "Content-Type": "application/json"})
        with controller.open(call, timeout=5):
            pass
        probe = subprocess.run(
            ["curl", "-fsS", "--noproxy", "", "--proxy-user", "termihomo:" + secret,
             "-x", "http://" + proxy, base + "/echo"], capture_output=True, timeout=5,
        )
        if node == "DIRECT":
            assert probe.returncode == 0 and probe.stdout == b"proxy-ok"
        else:
            assert probe.returncode != 0
        request(running)
    unauthenticated = subprocess.run(
        ["curl", "-sS", "--noproxy", "", "-o", os.devnull, "-w", "%{http_code}",
         "-x", "http://" + proxy, base + "/echo"], capture_output=True, timeout=5,
    )
    assert unauthenticated.stdout == b"407"


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


def cold_proxy_controller(binary, env, status):
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 27, 122, 0, 0))
    process = subprocess.Popen(
        [binary], stdin=slave, stdout=slave, stderr=slave, env=env, start_new_session=True,
    )
    try:
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.1)[0]:
                os.read(master, 65536)
            current = status()
            if current["state"] == "Paused":
                assert "proxy" not in current, current
                # Drain the UI until it has completed its controller bootstrap.
                while time.monotonic() < deadline:
                    os.write(master, b"q")
                    try:
                        assert process.wait(timeout=0.25) == 0
                        return current["pid"]
                    except subprocess.TimeoutExpired:
                        if select.select([master], [], [], 0)[0]:
                            os.read(master, 65536)
                break
        raise AssertionError("TUI did not initialize the proxy controller with the listener off")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(slave)
        os.close(master)


def terminal_smoke(binary, env, running, status, request, command, base):
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

    def click(column, row):
        key(f"\x1b[<0;{column};{row}M\x1b[<0;{column};{row}m".encode())

    instance = json.loads((Path(env["XDG_DATA_HOME"]) / "termihomo" / "instance.json").read_text())
    controller = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    config_request = urllib.request.Request(
        f"http://{running['controller']}/configs",
        headers={"Authorization": "Bearer " + instance["Secret"]},
    )

    def wait_for_mode(mode):
        deadline = time.monotonic() + 12
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.1)[0]:
                seen.extend(os.read(master, 65536))
            with controller.open(config_request, timeout=2) as response:
                actual = json.load(response)["mode"]
            if actual.lower() == mode:
                return
        raise AssertionError(f"terminal did not select mode {mode!r}")

    try:
        resize(122, 27)
        process = launch()
        expect("Running")
        key(b"m")
        wait_for_mode("global")
        key(b"2")
        expect("Current exit")
        key(b"1d")
        wait_for_mode("direct")
        # Probe the real mixed listener through an active private IPv4 interface.
        interfaces = json.loads(subprocess.check_output(["ip", "-j", "-4", "addr", "show", "up"])) if sys.platform == "linux" else []
        private = [ipaddress.ip_network(block) for block in ("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")]
        addresses = [
            addr["local"] for iface in interfaces if "BROADCAST" in iface["flags"]
            for addr in iface.get("addr_info", [])
            if any(ipaddress.ip_address(addr["local"]) in network for network in private)
        ]
        if addresses:
            lan_state = dict(running, proxy=f"{addresses[0]}:{running['proxy'].rsplit(':', 1)[1]}")
            key(b"w")
            expect("Allow devices on local subnets")
            key(b"y")
            expect(rb"\[w on\]|n\]")
            request(lan_state)
            key(b"w")
            expect("complete")
            try:
                with socket.create_connection((addresses[0], int(lan_state["proxy"].rsplit(":", 1)[1])), timeout=1):
                    raise AssertionError("LAN listener remained open after disabling")
            except OSError:
                pass
        key(b"b")
        wait_for_mode("rule")
        assert status()["pid"] == running["pid"]
        key(b"2")
        expect("REJECT")
        click(50, 7)  # Focus the second card, DIRECT.
        assert '"Pick"\tSelector\t"REJECT"' in command("proxy", "list").stdout
        key(b"\r")
        deadline = time.monotonic() + 12
        while '"Pick"\tSelector\t"DIRECT"' not in command("proxy", "list").stdout:
            assert time.monotonic() < deadline, "terminal did not select DIRECT"
            time.sleep(0.1)
        key(b"5[")
        expect("Mihomo logs")
        request(running)
        expect("Pick[DIRECT]")
        key(b"4")
        expect("Requests")
        expect("Destination")
        resize(37, 10)
        process.send_signal(signal.SIGWINCH)
        key(b"1")
        expect("Terminal too small")
        key(b"q")
        assert process.wait(timeout=5) == 0
        assert termios.tcgetattr(slave) == original
        assert status()["pid"] == running["pid"]
        request(running)
        resize(122, 27)
        process = launch("--lang", "zh-CN")
        expect("仪表盘")
        expect("运行中")
        key(b"3")
        expect("编辑")
        key(b"e")
        expect("编辑配置")
        expect("取消")
        click(43, 19)  # Cancel in the centered form.
        key(b"1")
        expect("停止")
        click(60, 6)  # Stop from the Overview status card.
        expect("确认操作")
        key(b"y")
        expect("[a ▶ 启动]")
        paused = status()
        assert paused["state"] == "Paused" and paused["pid"] == running["pid"] and "proxy" not in paused, paused
        host, port = running["proxy"].rsplit(":", 1)
        try:
            with socket.create_connection((host, int(port)), timeout=1):
                raise AssertionError("proxy listener remained open while paused")
        except OSError:
            pass
        key(b"2")
        expect("DIRECT")
        command("proxy", "select", "Pick", "REJECT")
        assert '"Pick"\tSelector\t"REJECT"' in command("proxy", "list").stdout
        assert " ms" in command("proxy", "test", "DIRECT", base + "/delay").stdout
        key(b"1")
        expect("[a ▶ 启动]")
        click(60, 6)  # Start from the same status card.
        expect("[a ■ 停止]")
        restarted = status()
        assert restarted["state"] == "Running" and restarted["pid"] == running["pid"]
        assert '"Pick"\tSelector\t"REJECT"' in command("proxy", "list").stdout
        key(b"q")
        assert process.wait(timeout=5) == 0
        command("proxy", "select", "Pick", "DIRECT")
        request(restarted)
        return restarted
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
