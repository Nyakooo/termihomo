#!/usr/bin/env python3
"""Refresh bundled geodata from verified official release assets (maintainer only)."""
import concurrent.futures
import gzip
import hashlib
import json
from pathlib import Path
import urllib.request

ROOT = Path(__file__).resolve().parents[1] / "internal/core/geodata"
NAMES = {
    "geoip.dat": "GeoIP.dat",
    "geoip.metadb": "geoip.metadb",
    "geosite.dat": "GeoSite.dat",
    "GeoLite2-ASN.mmdb": "ASN.mmdb",
}


def get(url):
    request = urllib.request.Request(url, headers={"User-Agent": "Termihomo-geodata-maintenance"})
    with urllib.request.urlopen(request, timeout=90) as response:
        body = response.read((64 << 20) + 1)
    if len(body) > 64 << 20:
        raise ValueError("Asset exceeds 64 MiB")
    return body


def fetch(asset):
    body = get(asset["browser_download_url"])
    digest = hashlib.sha256(body).hexdigest()
    if asset.get("digest") != "sha256:" + digest or len(body) != asset["size"]:
        raise ValueError("Release asset verification failed: " + asset["name"])
    compressed = gzip.compress(body, compresslevel=9, mtime=0)
    entry = dict(name=NAMES[asset["name"]], source=asset["browser_download_url"],
                 sha256=digest, size=len(body), compressed=len(compressed))
    return entry, compressed


def main():
    release = json.loads(get("https://api.github.com/repos/MetaCubeX/meta-rules-dat/releases/latest"))
    assets = {asset["name"]: asset for asset in release["assets"]}
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        results = list(pool.map(fetch, [assets[name] for name in NAMES]))
    base = "https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/master/"
    license_body, readme = get(base + "LICENSE"), get(base + "README.md")
    # Download and verify everything before changing the checked-in snapshot.
    for entry, compressed in results:
        (ROOT / (entry["name"] + ".gz")).write_bytes(compressed)
        print(entry["name"], entry["size"], entry["sha256"])
    (ROOT / "LICENSE").write_bytes(license_body)
    (ROOT / "UPSTREAM.md").write_text("\n".join(line.rstrip() for line in readme.decode().splitlines()).rstrip() + "\n")
    (ROOT / "manifest.json").write_text(json.dumps([entry for entry, _ in results], indent=2) + "\n")


if __name__ == "__main__":
    main()
