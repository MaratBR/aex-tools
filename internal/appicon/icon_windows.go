package appicon

import (
	"errors"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The icon of an exe, from its resources: at the size wanted, as 32-bit pixels.
var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	gdi32               = windows.NewLazySystemDLL("gdi32.dll")
	privateExtractIcons = user32.NewProc("PrivateExtractIconsW")
	getIconInfo         = user32.NewProc("GetIconInfo")
	destroyIcon         = user32.NewProc("DestroyIcon")
	getDC               = user32.NewProc("GetDC")
	releaseDC           = user32.NewProc("ReleaseDC")
	getObject           = gdi32.NewProc("GetObjectW")
	getDIBits           = gdi32.NewProc("GetDIBits")
	deleteObject        = gdi32.NewProc("DeleteObject")
)

type iconInfo struct {
	isIcon             int32
	xHotspot, yHotspot uint32
	mask, color        windows.Handle
}

type bitmap struct {
	typ, width, height, widthBytes int32
	planes, bitsPixel              uint16
	bits                           uintptr
}

type bitmapInfo struct {
	size                         uint32
	width, height                int32
	planes, bitCount             uint16
	compression, sizeImage       uint32
	xPelsPerMeter, yPelsPerMeter int32
	clrUsed, clrImportant        uint32
	colors                       [256]uint32 // room for a palette GetDIBits may write
}

func readIcon(path string, size int) (image.Image, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var icon windows.Handle
	var id uint32
	n, _, _ := privateExtractIcons.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(size), uintptr(size),
		uintptr(unsafe.Pointer(&icon)), uintptr(unsafe.Pointer(&id)), 1, 0)
	if n == 0 || n == 0xFFFFFFFF || icon == 0 {
		return nil, errors.New("no icon")
	}
	defer destroyIcon.Call(uintptr(icon))
	var ii iconInfo
	if r, _, err := getIconInfo.Call(uintptr(icon), uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil, err
	}
	defer deleteObject.Call(uintptr(ii.mask))
	if ii.color == 0 {
		return nil, errors.New("black and white icon")
	}
	defer deleteObject.Call(uintptr(ii.color))
	var bm bitmap
	if r, _, err := getObject.Call(uintptr(ii.color), unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil, err
	}
	w, h := int(bm.width), int(bm.height)
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
		return nil, errors.New("bad icon size")
	}
	dc, _, _ := getDC.Call(0)
	defer releaseDC.Call(0, dc)
	// pixels gives a bitmap's pixels top down, BGRA.
	pixels := func(b windows.Handle) ([]byte, error) {
		bi := bitmapInfo{width: int32(w), height: -int32(h), planes: 1, bitCount: 32}
		bi.size = uint32(unsafe.Offsetof(bi.colors))
		buf := make([]byte, w*h*4)
		if r, _, err := getDIBits.Call(dc, uintptr(b), 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), 0); r == 0 {
			return nil, err
		}
		return buf, nil
	}
	color, err := pixels(ii.color)
	if err != nil {
		return nil, err
	}
	hasAlpha := false
	for i := 3; i < len(color); i += 4 {
		if color[i] != 0 {
			hasAlpha = true
			break
		}
	}
	// An icon without alpha is see-through where its mask is white.
	var mask []byte
	if !hasAlpha {
		if mask, err = pixels(ii.mask); err != nil {
			return nil, err
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(color); i += 4 {
		a := color[i+3]
		if !hasAlpha {
			a = 255
			if mask[i] != 0 {
				a = 0
			}
		}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = color[i+2], color[i+1], color[i], a
	}
	return img, nil
}
