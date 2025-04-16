package events

type Message struct {
	Type string
	Data any
}

func NewMsgOfResponseDuration(data any) *Message {
	return &Message{
		Type: "responseDuration",
		Data: data,
	}
}
