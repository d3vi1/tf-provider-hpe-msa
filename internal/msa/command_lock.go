package msa

import "sync"

// commandMu serializes every MSA command issued by this provider process.
//
// The HPE MSA array's embedded XML API does not tolerate concurrent commands
// well: overlapping create/map/delete operations can conflict, and concurrent
// "map volume" auto-assign requests can race to pick the same free LUN. To make
// the provider safe at terraform -parallelism > 1, every command (login,
// logout, show, create, map, delete, ...) acquires this single global mutex for
// the full duration of its request+response cycle.
//
// The lock is taken at the lowest common execution point (getWithRetry), which
// every command funnels through exactly once per HTTP round-trip. No code path
// holds this lock while issuing another command, so it is never acquired
// re-entrantly within a single command.
var commandMu sync.Mutex
