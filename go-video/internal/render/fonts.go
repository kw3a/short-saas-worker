package render

import (
	"os"
	"sync"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

// Font paths. Montserrat ships with the binary (assets dir); DejaVu comes from
// the system font package installed in the container.
const (
	montserratRel  = "Montserrat-ExtraBold.ttf"
	dejaVuSans     = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	dejaVuSansBold = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
)

var (
	fontMu    sync.Mutex
	fontCache = map[string]*truetype.Font{}
)

func loadTTF(path string) (*truetype.Font, error) {
	fontMu.Lock()
	defer fontMu.Unlock()
	if f, ok := fontCache[path]; ok {
		return f, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := truetype.Parse(b)
	if err != nil {
		return nil, err
	}
	fontCache[path] = f
	return f, nil
}

// face returns a font.Face for the given TTF path and point size.
func face(path string, points float64) (font.Face, error) {
	f, err := loadTTF(path)
	if err != nil {
		return nil, err
	}
	return truetype.NewFace(f, &truetype.Options{Size: points}), nil
}
