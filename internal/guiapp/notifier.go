package guiapp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/gen2brain/beeep"
)

const notificationTitle = "NetGap"

var desktopNotificationIcon = makeNotificationIcon()

func makeNotificationIcon() []byte {
	const size = 32
	icon := image.NewRGBA(image.Rect(0, 0, size, size))
	red := color.RGBA{R: 230, G: 72, B: 79, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-16, y-16
			if dx*dx+dy*dy <= 225 {
				icon.Set(x, y, red)
			}
		}
	}
	for y := 7; y < 21; y++ {
		for x := 14; x < 18; x++ {
			icon.Set(x, y, white)
		}
	}
	for y := 24; y < 28; y++ {
		for x := 14; x < 18; x++ {
			icon.Set(x, y, white)
		}
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, icon); err != nil {
		return nil
	}
	return encoded.Bytes()
}

func notifyDesktop(title, message string) error {
	return beeep.Notify(title, message, desktopNotificationIcon)
}
