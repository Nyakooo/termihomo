# Minimal controller/listener fixture for managed startup and restart tests.
import http.server
import json
import os
import re
import socket
import sys
import threading

if '-t' in sys.argv:
    sys.exit(0)

body = open(sys.argv[sys.argv.index('-f') + 1]).read()
def scalar(text, key):
    return re.search(r'^' + re.escape(key) + r':\s*(.*?)\s*$', text, re.M).group(1).strip('"\'')

config = {'mixed-port': int(scalar(body, 'mixed-port')),
          'allow-lan': scalar(body, 'allow-lan') == 'true',
          'tun': {'enable': False}, 'mode': 'rule'}
listener = None

def accept_connections(sock):
    while True:
        try:
            conn, _ = sock.accept()
            conn.close()
        except OSError:
            return

def bind_listener():
    global listener
    if listener:
        try:
            listener.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        listener.close()
        listener = None
    if config['mixed-port']:
        listener = socket.socket()
        listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        listener.bind(('0.0.0.0' if config['allow-lan'] else '127.0.0.1', config['mixed-port']))
        listener.listen()
        threading.Thread(target=accept_connections, args=(listener,), daemon=True).start()

class Controller(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def respond(self, value):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(json.dumps(value).encode())

    def do_GET(self):
        if self.path == '/version':
            self.respond({'version': os.environ['TEST_NETWORK_KERNEL_VERSION']})
        elif self.path == '/rules':
            self.respond({'rules': [{'type': 'MATCH', 'proxy': 'DIRECT'}]})
        elif self.path == '/proxies':
            self.respond({'proxies': {}})
        elif self.path == '/configs':
            if os.environ.get('TEST_NETWORK_CONFIG_FAILURE') == '1':
                self.send_error(503)
            else:
                self.respond(config)
        else:
            self.send_error(404)

    def do_PATCH(self):
        patch = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        config.update(patch)
        bind_listener()
        self.respond({})

    def do_PUT(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        payload = request['payload']
        config.update({'mixed-port': int(scalar(payload, 'mixed-port')),
                       'allow-lan': scalar(payload, 'allow-lan') == 'true',
                       'mode': scalar(payload, 'mode')})
        bind_listener()
        self.respond({})

    def do_DELETE(self):
        self.respond({})

bind_listener()
host, port = scalar(body, 'external-controller').split(':')
http.server.HTTPServer((host, int(port)), Controller).serve_forever()
