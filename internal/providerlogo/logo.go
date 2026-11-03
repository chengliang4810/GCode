// Package providerlogo owns provider artwork independently of any desktop UI.
package providerlogo

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"strings"

	_ "github.com/fyne-io/image/ico"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const MaxBytes = 256 << 10

type Brand struct {
	ID, Name string
}

// This is an artwork palette, independent of provider protocols and endpoints.
func Brands() []Brand {
	return []Brand{
		{"deepseek-mark", "DeepSeek"}, {"qwen-mark", "通义千问"}, {"doubao-mark", "豆包"},
		{"zhipu-mark", "智谱"}, {"kimi-mark", "Kimi"}, {"minimax-mark", "MiniMax"},
		{"hunyuan-mark", "腾讯混元"}, {"wenxin-mark", "文心一言"}, {"spark-mark", "讯飞星火"},
		{"baichuan-mark", "百川"},
		{"openai-mark", "OpenAI"}, {"claude-mark", "Claude"}, {"grok-mark", "Grok"},
		{"cursor-mark", "Cursor"}, {"github-mark", "GitHub"}, {"antigravity-mark", "Antigravity"},
		{"devin-mark", "Devin"}, {"hermes-mark", "Hermes"}, {"opencode-mark", "OpenCode"},
		{"muse-mark", "Muse"}, {"pi-mark", "Pi"},
	}
}

func IsBuiltin(value string) bool {
	for _, brand := range Brands() {
		if brand.ID == value {
			return true
		}
	}
	return false
}

// A logo is either a built-in artwork ID or a self-contained, validated image.
// Keeping downloaded bytes in the value makes it durable and usable remotely.
func Validate(value string) error {
	if value == "" || IsBuiltin(value) {
		return nil
	}
	data, svg, err := ImageData(value)
	if err != nil {
		return err
	}
	if svg {
		return validateSVG(data)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" || !validSize(config) {
		return errors.New("invalid provider logo image")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return errors.New("invalid provider logo image")
	}
	return nil
}

func ImageData(value string) (data []byte, svg bool, err error) {
	prefix := "data:image/png;base64,"
	if strings.HasPrefix(value, "data:image/svg+xml;base64,") {
		prefix, svg = "data:image/svg+xml;base64,", true
	}
	if !strings.HasPrefix(value, prefix) || len(value) > base64.StdEncoding.EncodedLen(MaxBytes)+len(prefix) {
		return nil, false, errors.New("invalid provider logo")
	}
	data, err = base64.StdEncoding.DecodeString(strings.TrimPrefix(value, prefix))
	if err != nil || len(data) == 0 {
		return nil, false, errors.New("invalid provider logo encoding")
	}
	return data, svg, nil
}

func validSize(config image.Config) bool {
	return config.Width > 0 && config.Height > 0 &&
		config.Width <= 1024 && config.Height <= 1024
}

func normalize(data []byte) (value string, err error) {
	// ICO decoders can panic on malformed DIB payloads. Contain that boundary
	// after checking all allocation sizes, without affecting application state.
	defer func() {
		if recover() != nil {
			value, err = "", errors.New("cannot decode website logo")
		}
	}()
	if len(data) == 0 || len(data) > MaxBytes {
		return "", errors.New("website logo exceeds the size limit")
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("<")) {
		data, err = processSVG(data, true)
		if err != nil {
			return "", err
		}
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	if bytes.HasPrefix(data, []byte{0, 0, 1, 0}) && !boundedICO(data) {
		return "", errors.New("invalid website ICO logo")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || !validSize(config) {
		return "", errors.New("website logo is not a supported image")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("cannot decode website logo")
	}
	if config.Width > 256 || config.Height > 256 {
		width, height := config.Width, config.Height
		if width >= height {
			height, width = max(1, height*256/width), 256
		} else {
			width, height = max(1, width*256/height), 256
		}
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Src, nil)
		img = resized
	}
	var output bytes.Buffer
	if err := png.Encode(&output, img); err != nil || output.Len() > MaxBytes {
		return "", errors.New("cannot encode website logo")
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes()), nil
}

func boundedICO(data []byte) bool {
	if len(data) < 6 {
		return false
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 32 || len(data) < 6+count*16 {
		return false
	}
	total := uint64(0)
	position := uint64(6 + count*16)
	for i := 0; i < count; i++ {
		entry := data[6+i*16 : 22+i*16]
		size, offset := uint64(binary.LittleEndian.Uint32(entry[8:12])), uint64(binary.LittleEndian.Uint32(entry[12:16]))
		total += size
		// The codec consumes entries sequentially rather than seeking. Verify
		// the exact bytes it will decode, not just the declared entry offset.
		if size < 8 || total > 1<<20 || offset != position || offset+size > uint64(len(data)) {
			return false
		}
		position += size
		body := data[offset : offset+size]
		if bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
			config, err := png.DecodeConfig(bytes.NewReader(body))
			if err != nil || !validSize(config) || config.Width > 256 || config.Height > 256 {
				return false
			}
		} else {
			if len(body) < 40 {
				return false
			}
			width, height := binary.LittleEndian.Uint32(body[4:8]), binary.LittleEndian.Uint32(body[8:12])
			if width == 0 || width > 256 || height == 0 || height > 512 {
				return false
			}
		}
	}
	return true
}

// Website SVGs must be self-contained artwork, with no active content or
// external references. The native renderer never opens a web view for them.
func validateSVG(data []byte) error {
	_, err := processSVG(data, false)
	return err
}

// MyGo renders vector shapes and gradients but not SVG filters. Remove static
// filter definitions and references on import, while validating every token,
// including discarded content. Saved SVGs use the strict path above.
func processSVG(data []byte, removeFilters bool) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	root, count := false, 0
	filterDepth, depth := 0, 0
	changed := false
	for {
		token, err := decoder.Token()
		if err == io.EOF && root && depth == 0 {
			if !changed {
				return data, nil
			}
			if err := encoder.Flush(); err != nil || output.Len() > MaxBytes {
				return nil, errors.New("website logo exceeds the size limit")
			}
			return output.Bytes(), nil
		}
		if err != nil {
			return nil, errors.New("invalid website SVG logo")
		}
		count++
		if count > 10000 {
			return nil, errors.New("website SVG logo is too complex")
		}
		if directive, ok := token.(xml.Directive); ok && len(directive) > 0 {
			return nil, errors.New("website SVG logo contains a directive")
		}
		switch element := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				if root || element.Name.Local != "svg" {
					return nil, errors.New("website response is not an SVG logo")
				}
				root = true
			}
			depth++
			switch element.Name.Local {
			case "svg", "g", "path", "rect", "circle", "ellipse", "line", "polyline", "polygon",
				"defs", "linearGradient", "radialGradient", "stop", "clipPath", "mask", "title", "desc", "use", "symbol":
			case "filter":
				if !removeFilters {
					return nil, errors.New("website SVG logo contains unsupported content")
				}
			case "feDropShadow", "feGaussianBlur", "feOffset", "feFlood", "feComposite", "feBlend",
				"feMerge", "feMergeNode", "feColorMatrix", "feComponentTransfer", "feFuncR", "feFuncG", "feFuncB", "feFuncA":
				if filterDepth == 0 {
					return nil, errors.New("website SVG logo contains unsupported content")
				}
			default:
				return nil, errors.New("website SVG logo contains unsupported content")
			}
			attrs := make([]xml.Attr, 0, len(element.Attr))
			for _, attr := range element.Attr {
				value := strings.ToLower(strings.TrimSpace(attr.Value))
				if strings.HasPrefix(strings.ToLower(attr.Name.Local), "on") ||
					(attr.Name.Local == "href" && !strings.HasPrefix(value, "#")) ||
					(strings.Contains(value, "url(") && !strings.HasPrefix(value, "url(#")) {
					return nil, errors.New("website SVG logo contains an external reference")
				}
				if removeFilters && attr.Name.Local == "filter" {
					changed = true
					continue
				}
				// The encoder declares namespaces from resolved token names.
				if attr.Name.Space != "xmlns" && (attr.Name.Space != "" || attr.Name.Local != "xmlns") {
					attrs = append(attrs, attr)
				}
			}
			if filterDepth > 0 || element.Name.Local == "filter" {
				filterDepth++
				changed = true
				continue
			}
			element.Attr = attrs
			token = element
		case xml.EndElement:
			depth--
			if filterDepth > 0 {
				filterDepth--
				continue
			}
		default:
			if filterDepth > 0 {
				continue
			}
		}
		if removeFilters {
			if err := encoder.EncodeToken(token); err != nil || output.Len() > MaxBytes {
				return nil, errors.New("website logo exceeds the size limit")
			}
		}
	}
}
