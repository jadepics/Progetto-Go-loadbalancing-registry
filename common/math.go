package common

type AddArgs struct {
	A int
	B int
}

type AddReply struct {
	Sum  int
	From string // instance id
}
