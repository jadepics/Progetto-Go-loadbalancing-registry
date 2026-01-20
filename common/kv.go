package common

// Put writes/overwrites a key.
type PutArgs struct {
	Key   string
	Value string
}

type PutReply struct {
	OK         bool
	From       string // instance id che ha risposto
	RedirectTo string // se non-primary: host:port del primary (best effort)
}

// Get reads a key.
type GetArgs struct {
	Key string
}

type GetReply struct {
	Found bool
	Value string
	From  string // instance id che ha risposto
}

// Apply è la replica (primary -> backup).
type ApplyArgs struct {
	Seq   int64
	Key   string
	Value string
}

type ApplyReply struct {
	OK bool
}

// Snapshot: bootstrap dello stato (backup -> primary) all'avvio.
type SnapshotArgs struct{}

type SnapshotReply struct {
	Seq   int64
	State map[string]string
}
