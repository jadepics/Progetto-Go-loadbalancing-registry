package common

type EchoArgs struct {
	Msg string
}

type EchoReply struct {
	Msg  string
	From string // instance id
}
