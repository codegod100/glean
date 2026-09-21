version       = "0.1.0"
author        = "glean"
description   = "Nim port spike for glean: atproto, storage and the database layer"
license       = "MIT"
srcDir        = "."

requires "nim >= 2.2.0"
requires "bearssl"
requires "nimcrypto"
requires "chronos"
requires "websock"

# Probes that touch only this machine. Safe to run anywhere, including CI.
const offlineProbes = @[
  "sqlite_probe",
  "dpop_probe",
  "store_probe",
  "gleandb_probe",
  "stores_probe",
  "social_probe",
  "reconnect_probe",   # uses a local websocket server, not the network
]

# Probes that talk to real servers. Kept separate because a failure here can
# mean "bsky.social is having a bad day" rather than "the port is broken", and
# a test suite that cannot tell those apart stops being believed.
const networkProbes = @[
  "oauth_probe",
  "xrpc_probe",
  "jetstream_probe",
  "feedparser_probe",   # fixtures are offline, but it also fetches real feeds
]

proc runProbe(name: string) =
  echo "\n=== " & name
  exec "nim c -r --hints:off -d:ssl " & name & ".nim"

task test, "Run the probes that need no network":
  for probe in offlineProbes:
    runProbe(probe)
  echo "\nAll offline probes passed."

task testnet, "Run the probes that talk to real servers":
  for probe in networkProbes:
    runProbe(probe)
  echo "\nAll network probes passed."

task testall, "Run every probe":
  for probe in offlineProbes & networkProbes:
    runProbe(probe)
  echo "\nAll probes passed."

task build, "Compile every module without running anything":
  for probe in offlineProbes & networkProbes:
    exec "nim c --hints:off -d:ssl -c " & probe & ".nim"
  echo "\nEverything compiles."

task schema, "Regenerate atproto/schema.nim from the Go source":
  exec "python3 tools/extract_schema.py"
