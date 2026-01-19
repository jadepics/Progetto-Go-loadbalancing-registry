package common

// Instance describes a running server instance of a service.
// Addr should be reachable by clients (e.g. "echo1:9101" inside docker-compose network).
type Instance struct {
	ID     string            // unique instance id (e.g. "echo1")
	Addr   string            // host:port
	Weight int               // used by stateful/weighted load balancing
	Meta   map[string]string // optional metadata (e.g. {"zone":"A"})
}

type RegisterArgs struct {
	Service  string
	Instance Instance
}

type RegisterReply struct {
	OK bool
}

type DeregisterArgs struct {
	Service string
	ID      string
}

type DeregisterReply struct {
	OK bool
}

type LookupArgs struct {
	Service string
}

type LookupReply struct {
	Instances []Instance
}
