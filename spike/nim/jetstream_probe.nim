## Reads the live ATProto firehose.
##
##   nim c -r -d:ssl jetstream_probe.nim
##
## Jetstream is public, so unlike the OAuth and XRPC probes this one can check
## the real thing end to end. The point of interest is server-side filtering:
## an unfiltered subscription is the whole network, so a consumer that loses
## its query parameters still receives a healthy-looking torrent of events
## that are simply the wrong ones.

import std/[json, sequtils, sets, strformat, strutils, tables]
import pkg/chronos
import atproto/jetstream

var failures = 0

proc report(name: string, ok: bool, detail = "") =
  if not ok: inc failures
  let label = if ok: "PASS" else: "FAIL"
  if detail.len > 0: echo &"  {label}  {name}  -- {detail}"
  else: echo &"  {label}  {name}"

proc main() {.async.} =
  echo "Jetstream probe"
  echo ""

  # --- URL construction ---------------------------------------------------
  block:
    let cfg = JetstreamConfig(
      url: DefaultJetstream,
      wantedCollections: @["app.bsky.feed.post", "app.bsky.feed.like"],
      cursor: 123,
    )
    let u = subscribeUrl(cfg)
    report("adds /subscribe", "/subscribe" in u)
    # Repeated keys, not one comma-joined value: Jetstream reads these as a
    # multi-valued parameter and a joined string matches no collection.
    report("repeats wantedCollections per value",
           u.count("wantedCollections=") == 2, u.split('?')[^1])
    report("carries the cursor", "cursor=123" in u)

  block:
    let cfg = JetstreamConfig(url: "wss://example.test/subscribe")
    report("does not double up /subscribe",
           subscribeUrl(cfg).count("/subscribe") == 1)

  # --- parsing ------------------------------------------------------------
  block:
    let raw = """{"did":"did:plc:abc","time_us":1720000000000000,
      "kind":"commit","commit":{"rev":"3k","operation":"create",
      "collection":"app.bsky.feed.post","rkey":"xyz","cid":"bafy",
      "record":{"text":"hi","$type":"app.bsky.feed.post"}}}"""
    let ev = parseEvent(raw)
    report("parses a commit event", ev.kind == ekCommit and ev.did == "did:plc:abc")
    report("parses the operation", ev.commit.operation == coCreate)
    report("parses the record", ev.commit.record{"text"}.getStr == "hi")
    report("time_us survives as int64", ev.timeUs == 1720000000000000'i64,
           $ev.timeUs)

  block:
    # A delete carries no record; reading one must not crash a consumer.
    let ev = parseEvent("""{"did":"did:plc:a","time_us":1,"kind":"commit",
      "commit":{"operation":"delete","collection":"c","rkey":"r"}}""")
    report("a delete has no record",
           ev.commit.operation == coDelete and
           (ev.commit.record == nil or ev.commit.record.kind == JNull))

  # --- the live firehose --------------------------------------------------
  const wanted = "app.bsky.feed.post"
  var events: seq[Event]
  try:
    let cfg = JetstreamConfig(
      url: DefaultJetstream,
      wantedCollections: @[wanted],
    )
    events = await collectEvents(cfg, count = 25, timeout = 30.seconds)
  except CatchableError as e:
    report("connected to the live firehose", false, e.msg)
    echo ""
    echo "RESULT: could not reach Jetstream."
    quit 1

  report("received live events", events.len > 0, &"{events.len} events")
  if events.len == 0:
    echo ""
    echo "RESULT: no events received."
    quit 1

  report("every event has a DID", events.allIt(it.did.startsWith("did:")))
  report("cursors advance",
         events[^1].timeUs >= events[0].timeUs,
         &"{events[0].timeUs} -> {events[^1].timeUs}")

  # The real check: server-side filtering actually applied. Without it this
  # would be a mix of likes, follows, reposts and everything else.
  var collections: CountTable[string]
  for ev in events:
    if ev.kind == ekCommit:
      collections.inc ev.commit.collection
  let others = toSeq(collections.keys).filterIt(it != wanted and it.len > 0)
  report("filter applied server-side (only " & wanted & ")",
         others.len == 0,
         if others.len == 0: $collections.len & " collection"
         else: "also saw " & others.join(", "))

  let posts = events.filterIt(
    it.kind == ekCommit and it.commit.collection == wanted and
    it.commit.operation == coCreate and it.commit.record != nil)
  report("create events carry a typed record",
         posts.len == 0 or posts[0].commit.record{"$type"}.getStr == wanted,
         if posts.len > 0: posts[0].commit.record{"$type"}.getStr else: "none seen")

  # --- cursor replay ------------------------------------------------------
  # This is how a consumer catches up after downtime, so "the cursor is
  # accepted" is not enough -- it has to actually rewind. Replaying from an
  # earlier cursor must return events at or after it, including ones already
  # seen live.
  block:
    let rewindTo = events[0].timeUs
    let seenLive = events.mapIt(it.timeUs).toHashSet
    try:
      let cfg = JetstreamConfig(
        url: DefaultJetstream,
        wantedCollections: @[wanted],
        cursor: rewindTo,
      )
      let replayed = await collectEvents(cfg, count = 25, timeout = 30.seconds)
      report("replay from a cursor returns events", replayed.len > 0,
             &"{replayed.len} events")
      if replayed.len > 0:
        report("replayed events start at or after the cursor",
               replayed[0].timeUs >= rewindTo,
               &"cursor {rewindTo} -> first {replayed[0].timeUs}")
        # Proof it rewound rather than resuming live: the replay should
        # re-deliver timestamps from the first batch.
        let overlap = replayed.countIt(it.timeUs in seenLive)
        report("replay re-delivers already-seen events (a real rewind)",
               overlap > 0, &"{overlap} of {replayed.len} overlap")
    except CatchableError as e:
      report("replay from a cursor returns events", false, e.msg)

  echo ""
  if failures == 0:
    echo "RESULT: Jetstream works, with server-side filtering and replay confirmed."
  else:
    echo &"RESULT: {failures} check(s) failed."
    quit 1

when isMainModule:
  waitFor main()
