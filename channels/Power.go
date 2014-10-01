package channels

type PowerDevice interface {
	SetPower(state float64) error
}

type PowerChannel struct {
	baseChannel
	device PowerDevice
}

func NewPowerChannel(device PowerDevice) *PowerChannel {
	return &PowerChannel{baseChannel{
		protocol: "humidity",
	}, device}
}

func (c *PowerChannel) Set(state float64) error {
	c.device.SetPower(state)
	return nil
}

func (c *PowerChannel) SendState(state float64) error {
	return c.SendEvent("state", state)
}