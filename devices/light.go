package devices

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/ninjasphere/go-ninja/api"
	"github.com/ninjasphere/go-ninja/channels"
	"github.com/ninjasphere/go-ninja/logger"
	"github.com/ninjasphere/go-ninja/model"
)

var log = logger.GetLogger("LightDevice")

type LightDeviceState struct {
	OnOff      *bool                `json:"on-off,omitempty"`
	Color      *channels.ColorState `json:"color,omitempty"`
	Brightness *float64             `json:"brightness,omitempty"`
	Transition *int                 `json:"transition,omitempty"`
}

func (c *LightDeviceState) Clone() *LightDeviceState {
	text, _ := json.Marshal(c)
	state := &LightDeviceState{}
	json.Unmarshal(text, state)
	return state
}

type LightBatchChannel struct {
	light *LightDevice
}

func (c *LightBatchChannel) SetBatch(state *LightDeviceState) error {
	return c.light.SetBatch(state)
}

func (c *LightBatchChannel) GetProtocol() string {
	return "core/batching"
}

func (c *LightBatchChannel) SetEventHandler(_ func(event string, payload ...interface{}) error) {
}

type LightDevice struct {
	baseDevice
	sync.Mutex

	// ApplyLightState is required in your driver,
	// and should actually set the state on the physical light
	ApplyLightState func(state *LightDeviceState) error

	// The following three are optional, and are used instead of ApplyLightState
	// if only a single channel state is being set.
	ApplyOnOff      func(state bool) error
	ApplyBrightness func(state float64) error
	ApplyColor      func(state *channels.ColorState) error
	ApplyTransition func(state int) error
	ApplyIdentify   func() error

	// getter methods
	ApplyIsOn func() (bool, error)

	state      *LightDeviceState
	batch      bool // unused
	colorModes []string

	onOffChannel      *channels.OnOffChannel
	brightnessChannel *channels.BrightnessChannel
	colorChannel      *channels.ColorChannel
	transitionChannel *channels.TransitionChannel
	identifyChannel   *channels.IdentifyChannel
}

// UpdateLightState sends the new state(s) to the channel(s) to update the user interfaces
// LightDevice doesn't call this automatically, so you should call it as needed in your driver
func (d *LightDevice) UpdateLightState(state *LightDeviceState) error {
	d.state = state

	if state.OnOff != nil {
		if d.onOffChannel != nil {
			if err := d.onOffChannel.SendEvent("state", *state.OnOff); err != nil {
				return fmt.Errorf("Failed emitting on-off state: %s", err)
			}
		}
	}

	if state.Brightness != nil {
		if d.brightnessChannel != nil {
			if err := d.brightnessChannel.SendEvent("state", *state.Brightness); err != nil {
				return fmt.Errorf("Failed emitting brightness state: %s", err)
			}
		}
	}

	if state.Color != nil {
		if d.colorChannel != nil {
			if err := d.colorChannel.SendEvent("state", *state.Color); err != nil {
				return fmt.Errorf("Failed emitting color state: %s", err)
			}
		}
	}

	if state.Transition != nil {
		if d.transitionChannel != nil {
			if err := d.transitionChannel.SendEvent("state", *state.Transition); err != nil {
				return fmt.Errorf("Failed emitting transition state: %s", err)
			}
		}
	}

	return nil
}

// SetBatch runs when airwheel gesture is used
func (d *LightDevice) SetBatch(state *LightDeviceState) error {
	d.Lock()
	defer d.Unlock()

	// NOTE: changed following line and similar ones in other functions so it only updates the state just set
	// when it clones the existing state, it updates/sets on/off and brightness, which cancel each other out
	// since on = brightness 100%
	// this may not be the same for all bulbs (have an on/off state AND a brightness), so might need to be changed
	// or given a configuration option
	//	lightState := d.state.Clone()
	lightState := &LightDeviceState{}
	if state.OnOff != nil {
		lightState.OnOff = state.OnOff
	}
	if state.Brightness != nil {
		lightState.Brightness = state.Brightness
	}
	if state.Color != nil {
		lightState.Color = state.Color
	}
	if state.Transition != nil {
		lightState.Transition = state.Transition
	}

	return d.ApplyLightState(lightState)
}

func (d *LightDevice) SetOnOff(state bool) error {
	d.Lock()
	defer d.Unlock()

	var err error

	if state {
		d.log.Infof("Turning On")
	} else {
		d.log.Infof("Turning Off")
	}

	if d.ApplyOnOff != nil {
		err = d.ApplyOnOff(state)
	} else {
		//		lightState := d.state.Clone()
		lightState := &LightDeviceState{}
		lightState.OnOff = &state

		err = d.ApplyLightState(lightState)
	}

	return err
}

func (d *LightDevice) ToggleOnOff() error {
	if d.state.OnOff == nil {
		d.log.Warningf("On-off channel is in an unknown state for toggling. Setting to off.")
		return d.SetOnOff(false)
	}
	return d.SetOnOff(!*d.state.OnOff)
}

func (d *LightDevice) SetBrightness(state float64) error {

	if d.brightnessChannel == nil {
		return fmt.Errorf("This device does not have a brightness channel")
	}

	d.Lock()
	defer d.Unlock()

	var err error

	d.log.Infof("Setting brightness to %f", state)

	if d.ApplyBrightness != nil {
		err = d.ApplyBrightness(state)
	} else {
		//		lightState := d.state.Clone()
		lightState := &LightDeviceState{}
		lightState.Brightness = &state

		err = d.ApplyLightState(lightState)
	}

	return err
}

// SetColor sets the color for a light
// All modes are converted to hue then corresponding color values are stored in the ColorState
func (d *LightDevice) SetColor(state *channels.ColorState) error {
	if d.colorChannel == nil {
		return fmt.Errorf("This device does not have a color channel")
	}

	d.Lock()
	defer d.Unlock()

	var err error

	//	lightState := d.state.Clone()
	lightState := &LightDeviceState{}
	lightState.Color = state

	if !containsString(d.colorModes, state.Mode) {
		d.log.Debugf("Does not support the color mode: %s, so converting", state.Mode)

		if lightState.Brightness == nil {
			d.log.Warningf("No brightness value available, but can't convert without it, so defaulting to 1")
			brightness := float64(1)
			lightState.Brightness = &brightness
		}

		var color colorful.Color

		switch lightState.Color.Mode {
		case "hue":
			color = colorful.Hsv(*lightState.Color.Hue*float64(360), *lightState.Color.Saturation, *lightState.Brightness)
		case "temperature":
			color = temperatureToColor(float64(*lightState.Color.Temperature))
		case "xy":
			color = colorful.Xyy(*lightState.Color.X, *lightState.Color.Y, *lightState.Brightness)
		default:
			return fmt.Errorf("Unknown color mode: %s", lightState.Color.Mode)
		}

		h, c, _ := color.Hcl()
		lightState.Color = &channels.ColorState{
			Mode:       "hue",
			Hue:        &h,
			Saturation: &c,
		}
	}

	json, _ := json.Marshal(lightState)
	d.log.Infof("Setting Color to %s", json)

	if d.ApplyColor != nil {
		err = d.ApplyColor(state)
	} else {
		err = d.ApplyLightState(lightState)
	}

	return err
}

func (d *LightDevice) SetTransition(state int) error {
	if d.transitionChannel == nil {
		return fmt.Errorf("This device does not have a transition channel")
	}

	d.Lock()
	defer d.Unlock()

	d.state.Transition = &state

	var err error

	if d.ApplyTransition != nil {
		err = d.ApplyTransition(state)
	}
	// I don't think we'd ever want to send a full state to the bulb if we are only updating the transition time
	return err
}

func (d *LightDevice) Identify() error {
	if d.ApplyIdentify == nil {
		return fmt.Errorf("Identify is not enabled on this device")
	}
	return d.ApplyIdentify()
}

// IsOn is for determining if the light is on or off
func (d *LightDevice) IsOn() (bool, error) {
	if d.ApplyIsOn == nil {
		return false, fmt.Errorf("IsOn is not enabled on this device")
	}
	return d.ApplyIsOn()
}

func (d *LightDevice) EnableOnOffChannel() error {
	d.onOffChannel = channels.NewOnOffChannel(d)
	return d.conn.ExportChannel(d, d.onOffChannel, "on-off")
}

func (d *LightDevice) EnableBrightnessChannel() error {
	d.brightnessChannel = channels.NewBrightnessChannel(d)
	return d.conn.ExportChannel(d, d.brightnessChannel, "brightness")
}

func (d *LightDevice) EnableIdentifyChannel() error {
	if d.ApplyIdentify == nil {
		return fmt.Errorf("If you want to enable the identify channel, you must provide an applyIdentify function")
	}
	d.identifyChannel = channels.NewIdentifyChannel(d)
	return d.conn.ExportChannel(d, d.identifyChannel, "identify")
}

func (d *LightDevice) EnableColorChannel(supportedModes ...string) error {
	if len(supportedModes) == 0 {
		log.Errorf("You must support at least one color mode")
	}
	if !containsString(supportedModes, "hue") {
		log.Errorf("You must support at least hue color values")
	}
	d.colorModes = supportedModes
	d.colorChannel = channels.NewColorChannel(d)
	return d.conn.ExportChannel(d, d.colorChannel, "color")
}

func (d *LightDevice) EnableTransitionChannel() error {
	d.transitionChannel = channels.NewTransitionChannel(d)
	return d.conn.ExportChannel(d, d.transitionChannel, "transition")
}

// CreateLightDevice returns a LightDevice for a driver to use
func CreateLightDevice(driver ninja.Driver, info *model.Device, conn *ninja.Connection) (*LightDevice, error) {

	d := &LightDevice{
		baseDevice: baseDevice{
			conn:   conn,
			driver: driver,
			log:    logger.GetLogger("LightDevice - " + *info.Name),
			info:   info,
		},
	}

	err := conn.ExportDevice(d)
	if err != nil {
		//		d.log.Errorf("Failed to export device %s!", *info.Name, err)
		return nil, fmt.Errorf("Failed to export device %s!", *info.Name, err)
	}

	methods := []string{"setBatch"}
	events := []string{}

	err = conn.ExportChannelWithSupported(d, &LightBatchChannel{d}, "batch", &methods, &events)
	if err != nil {
		//		d.log.Fatalf("Failed to create batch channel: %s", err)
		return nil, fmt.Errorf("Failed to create batch channel: %s", err)
	}

	d.log.Infof("Created LightDevice %s", info.Name)

	return d, nil
}

// from http://www.tannerhelland.com/4435/convert-temperature-rgb-algorithm-code/
func temperatureToColor(Temperature float64) colorful.Color {

	Temperature = Temperature / 100

	//Calculate Red:
	var Red float64

	if Temperature <= 66 {
		Red = 255
	} else {
		Red = Temperature - 60
		Red = 329.698727446 * math.Pow(Red, -0.1332047592)
		if Red < 0 {
			Red = 0
		}
		if Red > 255 {
			Red = 255
		}
	}

	//Calculate Green:
	var Green float64

	if Temperature <= 66 {
		Green = Temperature
		Green = 99.4708025861*math.Log(Green) - 161.1195681661
		if Green < 0 {
			Green = 0
		}
		if Green > 255 {
			Green = 255
		}
	} else {
		Green = Temperature - 60
		Green = 288.1221695283 * math.Pow(Green, -0.0755148492)
		if Green < 0 {
			Green = 0
		}
		if Green > 255 {
			Green = 255
		}
	}

	//Calculate Blue:
	var Blue float64

	if Temperature >= 66 {
		Blue = 255
	} else if Temperature <= 19 {

		Blue = 0
	} else {
		Blue = Temperature - 10
		Blue = 138.5177312231*math.Log(Blue) - 305.0447927307
		if Blue < 0 {
			Blue = 0
		}
		if Blue > 255 {
			Blue = 255
		}
	}

	return colorful.Color{Red / 255.0, Green / 255.0, Blue / 255.0}
}

func containsString(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}
