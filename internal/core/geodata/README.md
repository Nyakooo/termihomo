# Bundled GeoIP / GeoSite data

These snapshots come from [MetaCubeX/meta-rules-dat](https://github.com/MetaCubeX/meta-rules-dat).
`manifest.json` records the original download URLs, uncompressed sizes, and SHA-256
values verified against GitHub release asset digests. `LICENSE` and `UPSTREAM.md`
preserve the upstream license and dataset/source attribution. ASN data originates
from [MaxMind GeoLite2](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/).

All four formats are compressed with deterministic gzip and embedded into every
Termihomo executable, including Windows and Android. Installation/validation
extracts missing or incomplete files locally. Profile import does not download
these assets. Existing complete cached versions are retained.

To refresh the checked-in snapshot explicitly:

```sh
python3 scripts/update-geodata.py
go test ./internal/core
```

Review the resulting manifest, data, and upstream attribution together. Updating
this snapshot increases the executable size by approximately 14 MB. Subscription
YAML and any profile-specific proxy/rule providers remain independent resources;
providers may still need network access when Mihomo first initializes them.
