package agent

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
)

// jpegOrientation reads only EXIF's fixed-width orientation tag. Offsets are
// checked before every access; missing or malformed metadata has orientation 1.
func jpegOrientation(data []byte) int {
	if len(data) < 2 || data[0] != 0xff || data[1] != 0xd8 {
		return 1
	}
	for at := 2; at+4 <= len(data); {
		if data[at] != 0xff {
			return 1
		}
		marker := data[at+1]
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0xff {
			at++
			continue
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			at += 2
			continue
		}
		length := int(binary.BigEndian.Uint16(data[at+2 : at+4]))
		if length < 2 || length > len(data)-at-2 {
			return 1
		}
		payload := data[at+4 : at+2+length]
		if marker == 0xe1 && bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			return tiffOrientation(payload[6:])
		}
		at += 2 + length
	}
	return 1
}

func tiffOrientation(data []byte) int {
	if len(data) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(data[2:4]) != 42 {
		return 1
	}
	offset := uint64(order.Uint32(data[4:8]))
	if offset+2 > uint64(len(data)) {
		return 1
	}
	count := uint64(order.Uint16(data[offset : offset+2]))
	start := offset + 2
	if count > (uint64(len(data))-start)/12 {
		return 1
	}
	for i := uint64(0); i < count; i++ {
		entry := data[start+i*12 : start+(i+1)*12]
		if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 &&
			order.Uint32(entry[4:8]) == 1 {
			value := int(order.Uint16(entry[8:10]))
			if value >= 1 && value <= 8 {
				return value
			}
		}
	}
	return 1
}

type orientedImage struct {
	image.Image
	orientation int
}

func orientImage(source image.Image, orientation int) image.Image {
	if orientation <= 1 || orientation > 8 {
		return source
	}
	return orientedImage{Image: source, orientation: orientation}
}

func (o orientedImage) Bounds() image.Rectangle {
	size := o.Image.Bounds().Size()
	if o.orientation >= 5 {
		size.X, size.Y = size.Y, size.X
	}
	return image.Rect(0, 0, size.X, size.Y)
}

func (o orientedImage) At(x, y int) color.Color {
	bounds := o.Image.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	switch o.orientation {
	case 2:
		x = width - 1 - x
	case 3:
		x, y = width-1-x, height-1-y
	case 4:
		y = height - 1 - y
	case 5:
		x, y = y, x
	case 6:
		x, y = y, height-1-x
	case 7:
		x, y = width-1-y, height-1-x
	case 8:
		x, y = width-1-y, x
	}
	return o.Image.At(bounds.Min.X+x, bounds.Min.Y+y)
}

func (o orientedImage) Opaque() bool {
	opaque, ok := o.Image.(interface{ Opaque() bool })
	return ok && opaque.Opaque()
}
