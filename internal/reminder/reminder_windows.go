package reminder

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"aex/internal/browser"
)

// On Windows a reminder is a card on every screen (a topmost Win32 window per monitor, drawn with
// GDI), in the upper part of its work area. It never takes the focus, so typing elsewhere goes on;
// its × on any screen closes it on all of them. Links in the message open in their browser
// (internal/browser) and leave it open, unless marked to close it (CloseMark). Each reminder has a thread of its own running its
// windows' message loop; reminders shown at once are cascaded so each can be seen.

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")
	winmm  = windows.NewLazySystemDLL("winmm.dll")

	pRegisterClassEx              = user32.NewProc("RegisterClassExW")
	pCreateWindowEx               = user32.NewProc("CreateWindowExW")
	pDefWindowProc                = user32.NewProc("DefWindowProcW")
	pDestroyWindow                = user32.NewProc("DestroyWindow")
	pShowWindow                   = user32.NewProc("ShowWindow")
	pSetWindowPos                 = user32.NewProc("SetWindowPos")
	pGetMessage                   = user32.NewProc("GetMessageW")
	pTranslateMessage             = user32.NewProc("TranslateMessage")
	pDispatchMessage              = user32.NewProc("DispatchMessageW")
	pPostQuitMessage              = user32.NewProc("PostQuitMessage")
	pEnumDisplayMonitors          = user32.NewProc("EnumDisplayMonitors")
	pGetMonitorInfo               = user32.NewProc("GetMonitorInfoW")
	pBeginPaint                   = user32.NewProc("BeginPaint")
	pEndPaint                     = user32.NewProc("EndPaint")
	pGetClientRect                = user32.NewProc("GetClientRect")
	pInvalidateRect               = user32.NewProc("InvalidateRect")
	pFillRect                     = user32.NewProc("FillRect")
	pDrawText                     = user32.NewProc("DrawTextW")
	pGetDC                        = user32.NewProc("GetDC")
	pReleaseDC                    = user32.NewProc("ReleaseDC")
	pLoadCursor                   = user32.NewProc("LoadCursorW")
	pSetCursor                    = user32.NewProc("SetCursor")
	pGetCursorPos                 = user32.NewProc("GetCursorPos")
	pScreenToClient               = user32.NewProc("ScreenToClient")
	pTrackMouseEvent              = user32.NewProc("TrackMouseEvent")
	pSetThreadDpiAwarenessContext = user32.NewProc("SetThreadDpiAwarenessContext")
	pGetDpiForMonitor             = shcore.NewProc("GetDpiForMonitor")
	pDwmSetWindowAttribute        = dwmapi.NewProc("DwmSetWindowAttribute")
	pPlaySound                    = winmm.NewProc("PlaySoundW")

	pCreateFont             = gdi32.NewProc("CreateFontW")
	pCreateSolidBrush       = gdi32.NewProc("CreateSolidBrush")
	pCreateCompatibleDC     = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatibleBitmap = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt                 = gdi32.NewProc("BitBlt")
	pDeleteDC               = gdi32.NewProc("DeleteDC")
	pSelectObject           = gdi32.NewProc("SelectObject")
	pDeleteObject           = gdi32.NewProc("DeleteObject")
	pGetStockObject         = gdi32.NewProc("GetStockObject")
	pSetBkMode              = gdi32.NewProc("SetBkMode")
	pSetTextColor           = gdi32.NewProc("SetTextColor")
	pRoundRect              = gdi32.NewProc("RoundRect")
	pGetTextExtentPoint32   = gdi32.NewProc("GetTextExtentPoint32W")
	pEllipse                = gdi32.NewProc("Ellipse")
)

const (
	wsPopup         = 0x80000000
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	swShowNoActive  = 4
	swpNoActivate   = 0x0010
	swpNoMove       = 0x0002
	swpNoSize       = 0x0001
	hwndTopmost     = ^uintptr(0) // -1
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmEraseBkgnd    = 0x0014
	wmSetCursor     = 0x0020
	wmMouseActivate = 0x0021
	wmMouseMove     = 0x0200
	wmLButtonUp     = 0x0202
	wmMouseLeave    = 0x02A3
	maNoActivate    = 3
	tmeLeave        = 0x00000002
	idcArrow        = 32512
	idcHand         = 32649
	dtCenter        = 0x0001
	dtRight         = 0x0002
	dtVCenter       = 0x0004
	dtSingleLine    = 0x0020
	dtNoPrefix      = 0x0800
	dtEndEllipsis   = 0x8000
	transparentBk   = 1
	nullPen         = 8
	srcCopy         = 0x00CC0020
	fwNormal        = 400
	fwSemibold      = 600
	cleartype       = 5
	sndAsync        = 0x0001
	sndNoDefault    = 0x0002
	sndMemory       = 0x0004
	mdtEffectiveDPI = 0
	// DWM: rounded corners (Windows 11) and the border's color.
	dwmaCornerPreference = 33
	dwmaBorderColor      = 34
	dwmCornerRound       = 2
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2: sizes in real pixels on each monitor.
	dpiPerMonitorV2 = ^uintptr(3) // -4
)

type rect struct{ Left, Top, Right, Bottom int32 }

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type paintStruct struct {
	Hdc       windows.Handle
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Hwnd      windows.HWND
	HoverTime uint32
}

// colors of a card, as COLORREFs (0x00BBGGRR).
type palette struct{ bg, text, dim, accent, accentHover, hoverBg, urgent uintptr }

func rgb(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

// The window's colors (tokens.css), light or dark as Windows' app mode.
var (
	lightColors = palette{rgb(255, 255, 255), rgb(29, 29, 31), rgb(110, 110, 115), rgb(0, 113, 227), rgb(0, 94, 190), rgb(232, 232, 237), rgb(215, 0, 21)}
	darkColors  = palette{rgb(44, 44, 46), rgb(245, 245, 247), rgb(152, 152, 157), rgb(10, 132, 255), rgb(64, 156, 255), rgb(58, 58, 60), rgb(255, 69, 58)}
)

func colors() palette {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return darkColors
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue("AppsUseLightTheme"); err == nil && v == 1 {
		return lightColors
	}
	return darkColors
}

const className = "aexReminder"

var (
	classOnce sync.Once
	classErr  error
	instance  windows.Handle

	cardsMu sync.Mutex
	cards   = map[windows.HWND]*card{}

	slotsMu sync.Mutex
	slots   []bool // cascade places in use
)

func register() error {
	classOnce.Do(func() {
		if classErr = windows.GetModuleHandleEx(0, nil, &instance); classErr != nil {
			return
		}
		name, _ := windows.UTF16PtrFromString(className)
		wc := wndClassEx{Style: 3 /* CS_HREDRAW | CS_VREDRAW */, WndProc: syscall.NewCallback(wndProc), Instance: instance, ClassName: name}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			classErr = err
		}
	})
	return classErr
}

// shown is one reminder's cards, one per monitor.
type shown struct {
	r     Reminder
	parts []Part
	at    string // when it appeared, e.g. 14:05
	slot  int
	colrs palette
	open  int // cards not destroyed yet
}

// mark is the color of the card's border, dot and title: red for an urgent reminder.
func (s *shown) mark() uintptr {
	if s.r.Urgent {
		return s.colrs.urgent
	}
	return s.colrs.accent
}

// run is a piece of the message's text placed on a card: a word, spaces, or a part of a word too
// long for a line; link is its part's index in shown.parts when that is a link, else -1.
type run struct {
	text    string
	x, y, w int32
	link    int
}

// card is a reminder's window on one monitor, laid out for its DPI.
type card struct {
	s                   *shown
	dpi                 int
	w, h                int32
	title, msg, close   rect // where each goes
	runs                []run
	lineH               int32
	hoverClose          bool
	hoverLink           int // the part hovered, -1 for none
	fontTitle, fontMsg  uintptr
	fontLink, fontSmall uintptr
	fontIcon            uintptr
}

func open(r Reminder) (<-chan struct{}, error) {
	if err := register(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	started := make(chan error, 1)
	go func() {
		// Windows belong to the thread that made them, which runs their message loop.
		runtime.LockOSThread()
		defer close(done)
		s := &shown{r: r, parts: r.Parts(), at: time.Now().Format("15:04"), slot: takeSlot(), colrs: colors()}
		defer freeSlot(s.slot)
		if pSetThreadDpiAwarenessContext.Find() == nil {
			pSetThreadDpiAwarenessContext.Call(dpiPerMonitorV2)
		}
		for _, m := range monitors() {
			if hwnd := s.create(m); hwnd != 0 {
				s.open++
			}
		}
		if s.open == 0 {
			started <- fmt.Errorf("could not open the reminder's windows: %w", syscall.GetLastError())
			return
		}
		started <- nil
		announce(r.Urgent, done, playChime, stopChime)
		var m msg
		for {
			if r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return done, nil
}

func takeSlot() int {
	slotsMu.Lock()
	defer slotsMu.Unlock()
	for i, used := range slots {
		if !used {
			slots[i] = true
			return i
		}
	}
	slots = append(slots, true)
	return len(slots) - 1
}

func freeSlot(i int) {
	slotsMu.Lock()
	slots[i] = false
	slotsMu.Unlock()
}

type monitor struct {
	handle uintptr
	work   rect
}

var (
	monitorsMu sync.Mutex
	found      []monitor
	enumProc   = syscall.NewCallback(func(h, _, _, _ uintptr) uintptr {
		mi := monitorInfo{}
		mi.Size = uint32(unsafe.Sizeof(mi))
		if r, _, _ := pGetMonitorInfo.Call(h, uintptr(unsafe.Pointer(&mi))); r != 0 {
			found = append(found, monitor{h, mi.Work})
		}
		return 1
	})
)

// monitors lists the screens with their work areas (without the taskbar), in the calling thread's
// DPI awareness.
func monitors() []monitor {
	monitorsMu.Lock()
	defer monitorsMu.Unlock()
	found = nil
	pEnumDisplayMonitors.Call(0, 0, enumProc, 0)
	return found
}

func dpiOf(m monitor) int {
	var x, y uint32
	if pGetDpiForMonitor.Find() == nil {
		if r, _, _ := pGetDpiForMonitor.Call(m.handle, mdtEffectiveDPI, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y))); r == 0 && x > 0 {
			return int(x)
		}
	}
	return 96
}

func font(face string, px int32, weight int, underline bool) uintptr {
	name, _ := windows.UTF16PtrFromString(face)
	u := uintptr(0)
	if underline {
		u = 1
	}
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, u, 0, 1 /* DEFAULT_CHARSET */, 0, 0, cleartype, 0, uintptr(unsafe.Pointer(name)))
	return f
}

// The close icon: ChromeClose in Segoe MDL2 Assets (Windows 10 and later).
const closeGlyph = ""

// create lays the card out for monitor m and shows it there; 0 when it could not be made.
func (s *shown) create(m monitor) windows.HWND {
	c := &card{s: s, dpi: dpiOf(m), hoverLink: -1}
	px := func(v int) int32 { return int32(v * c.dpi / 96) }
	c.fontTitle, c.fontSmall = font("Segoe UI", px(14), fwSemibold, false), font("Segoe UI", px(13), fwNormal, false)
	c.fontMsg, c.fontLink = font("Segoe UI", px(19), fwNormal, false), font("Segoe UI", px(19), fwNormal, true)
	c.fontIcon = font("Segoe MDL2 Assets", px(12), fwNormal, false)

	pad, width := px(24), px(440)
	c.w = width
	closeSize, closeGap := px(32), px(12)
	c.close = rect{width - closeGap - closeSize, closeGap, width - closeGap, closeGap + closeSize}
	cy := (c.close.Top + c.close.Bottom) / 2
	c.title = rect{pad + px(16), cy - px(10), c.close.Left - px(8), cy + px(10)}
	top := max(c.title.Bottom, c.close.Bottom) + px(8)
	c.msg = rect{pad, top, width - pad, top + c.layout(width-2*pad, px(420))}
	c.h = c.msg.Bottom + pad

	workW, workH := m.work.Right-m.work.Left, m.work.Bottom-m.work.Top
	step := px(28) * int32(s.slot%8)
	x := m.work.Left + (workW-c.w)/2 + step
	y := m.work.Top + max(px(16), workH/5-c.h/4) + step

	cls, _ := windows.UTF16PtrFromString(className)
	name, _ := windows.UTF16PtrFromString("aex: " + s.r.Heading())
	r, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolWindow|wsExNoActivate, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)),
		wsPopup, uintptr(x), uintptr(y), uintptr(c.w), uintptr(c.h), 0, 0, uintptr(instance), 0)
	hwnd := windows.HWND(r)
	if hwnd == 0 {
		c.free()
		return 0
	}
	cardsMu.Lock()
	cards[hwnd] = c
	cardsMu.Unlock()
	corner, border := uint32(dwmCornerRound), uint32(s.mark())
	if pDwmSetWindowAttribute.Find() == nil {
		pDwmSetWindowAttribute.Call(uintptr(hwnd), dwmaCornerPreference, uintptr(unsafe.Pointer(&corner)), 4)
		pDwmSetWindowAttribute.Call(uintptr(hwnd), dwmaBorderColor, uintptr(unsafe.Pointer(&border)), 4)
	}
	pShowWindow.Call(uintptr(hwnd), swShowNoActive)
	pSetWindowPos.Call(uintptr(hwnd), hwndTopmost, 0, 0, 0, 0, swpNoActivate|swpNoMove|swpNoSize)
	return hwnd
}

// layout places the message's parts in lines width wide, wrapping at spaces (and inside a word too
// long for a line), and returns the height they take, at most maxH: lines below it are left out.
func (c *card) layout(width, maxH int32) int32 {
	dc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, dc)
	old, _, _ := pSelectObject.Call(dc, c.fontMsg)
	defer pSelectObject.Call(dc, old)
	measure := func(s string) int32 {
		u, _ := windows.UTF16FromString(s)
		var size struct{ CX, CY int32 }
		pGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&size)))
		c.lineH = max(c.lineH, size.CY)
		return size.CX
	}
	measure("Ag")
	var x, y int32
	place := func(text string, w int32, link int) {
		c.runs = append(c.runs, run{text, x, y, w, link})
		x += w
	}
	newline := func() { x, y = 0, y+c.lineH }
	for i, p := range c.s.parts {
		link := -1
		if p.Link != "" {
			link = i
		}
		for _, tok := range tokens(p.Text) {
			switch {
			case tok == "\n":
				newline()
			case strings.TrimSpace(tok) == "":
				// Spaces where a line wraps are left out.
				if w := measure(tok); x > 0 && x+w <= width {
					place(tok, w, link)
				} else if x > 0 {
					newline()
				}
			default:
				w := measure(tok)
				if x > 0 && x+w > width {
					// The spaces before the break go too (a link's would show underlined).
					for n := len(c.runs); n > 0 && c.runs[n-1].y == y && strings.TrimSpace(c.runs[n-1].text) == ""; n-- {
						c.runs = c.runs[:n-1]
					}
					newline()
				}
				// A word longer than a line: as many characters on each line as fit.
				for w > width {
					rs := []rune(tok)
					n := len(rs) - 1
					for n > 1 && measure(string(rs[:n])) > width {
						n--
					}
					place(string(rs[:n]), measure(string(rs[:n])), link)
					newline()
					tok = string(rs[n:])
					w = measure(tok)
				}
				place(tok, w, link)
			}
		}
	}
	lines := max(maxH/c.lineH, 1)
	c.runs = slices.DeleteFunc(c.runs, func(r run) bool { return r.y/c.lineH >= lines })
	return min(y+c.lineH, lines*c.lineH)
}

// tokens splits text into words, runs of spaces and line breaks.
func tokens(text string) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	kind := func(r rune) int {
		switch {
		case r == '\n':
			return 2
		case unicode.IsSpace(r):
			return 1
		}
		return 0
	}
	var out []string
	start, last := 0, -1
	for i, r := range text {
		k := kind(r)
		if i > start && (k != last || k == 2) {
			out = append(out, text[start:i])
			start = i
		}
		last = k
	}
	if start < len(text) {
		out = append(out, text[start:])
	}
	return out
}

func drawText(dc uintptr, text string, r *rect, format uintptr) {
	s, _ := windows.UTF16FromString(text)
	pDrawText.Call(dc, uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)-1), uintptr(unsafe.Pointer(r)), format)
}

func (c *card) free() {
	for _, f := range []uintptr{c.fontTitle, c.fontMsg, c.fontLink, c.fontSmall, c.fontIcon} {
		if f != 0 {
			pDeleteObject.Call(f)
		}
	}
}

func (c *card) paint(dc uintptr) {
	p := c.s.colrs
	fill := func(r rect, color uintptr) {
		b, _, _ := pCreateSolidBrush.Call(color)
		pFillRect.Call(dc, uintptr(unsafe.Pointer(&r)), b)
		pDeleteObject.Call(b)
	}
	shape := func(proc *windows.LazyProc, r rect, color uintptr, radius int32) {
		b, _, _ := pCreateSolidBrush.Call(color)
		oldB, _, _ := pSelectObject.Call(dc, b)
		pen, _, _ := pGetStockObject.Call(nullPen)
		oldP, _, _ := pSelectObject.Call(dc, pen)
		if proc == pRoundRect {
			proc.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
		} else {
			proc.Call(dc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
		}
		pSelectObject.Call(dc, oldB)
		pSelectObject.Call(dc, oldP)
		pDeleteObject.Call(b)
	}
	text := func(f uintptr, color uintptr, s string, r rect, format uintptr) {
		old, _, _ := pSelectObject.Call(dc, f)
		pSetTextColor.Call(dc, color)
		drawText(dc, s, &r, format|dtNoPrefix)
		pSelectObject.Call(dc, old)
	}
	px := func(v int) int32 { return int32(v * c.dpi / 96) }

	fill(rect{0, 0, c.w, c.h}, p.bg)
	pSetBkMode.Call(dc, transparentBk)
	// Title line: a dot, the title, and when it appeared on the right; the close button after it.
	dot := px(9)
	cy := (c.title.Top + c.title.Bottom) / 2
	shape(pEllipse, rect{c.title.Left - px(16), cy - dot/2, c.title.Left - px(16) + dot, cy - dot/2 + dot}, c.s.mark(), 0)
	text(c.fontSmall, p.dim, c.s.at, c.title, dtRight|dtVCenter|dtSingleLine)
	head := c.title
	head.Right -= px(48)
	text(c.fontTitle, c.s.mark(), c.s.r.Heading(), head, dtVCenter|dtSingleLine|dtEndEllipsis)
	icon := p.dim
	if c.hoverClose {
		shape(pRoundRect, c.close, p.hoverBg, px(8))
		icon = p.text
	}
	text(c.fontIcon, icon, closeGlyph, c.close, dtCenter|dtVCenter|dtSingleLine)
	// The message, its links underlined in the accent color.
	for _, r := range c.runs {
		f, color := c.fontMsg, p.text
		if r.link >= 0 {
			f, color = c.fontLink, p.accent
			if r.link == c.hoverLink {
				color = p.accentHover
			}
		}
		x, y := c.msg.Left+r.x, c.msg.Top+r.y
		text(f, color, r.text, rect{x, y, x + r.w + px(2), y + c.lineH}, dtSingleLine)
	}
}

// cursor is where the mouse is on the card.
func cursor(hwnd windows.HWND) point {
	var pt point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
	return pt
}

func (r rect) has(pt point) bool {
	return pt.X >= r.Left && pt.X < r.Right && pt.Y >= r.Top && pt.Y < r.Bottom
}

// linkAt is the part whose link is at pt, -1 for none.
func (c *card) linkAt(pt point) int {
	for _, r := range c.runs {
		x, y := c.msg.Left+r.x, c.msg.Top+r.y
		if r.link >= 0 && (rect{x, y, x + r.w, y + c.lineH}).has(pt) {
			return r.link
		}
	}
	return -1
}

// hover updates what the mouse is over, redrawing when that changed.
func (c *card) hover(hwnd windows.HWND, pt point, inside bool) {
	onClose, link := inside && c.close.has(pt), -1
	if inside {
		link = c.linkAt(pt)
	}
	if onClose != c.hoverClose || link != c.hoverLink {
		c.hoverClose, c.hoverLink = onClose, link
		pInvalidateRect.Call(uintptr(hwnd), 0, 0)
	}
}

// dismiss closes every card of c's reminder; they all run on this thread.
func (c *card) dismiss() {
	cardsMu.Lock()
	var mine []windows.HWND
	for h, other := range cards {
		if other.s == c.s {
			mine = append(mine, h)
		}
	}
	cardsMu.Unlock()
	for _, h := range mine {
		pDestroyWindow.Call(uintptr(h))
	}
}

func wndProc(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
	cardsMu.Lock()
	c := cards[hwnd]
	cardsMu.Unlock()
	if c == nil {
		r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r
	}
	switch message {
	case wmMouseActivate:
		return maNoActivate // clicks never take the focus from the window being typed in
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		var ps paintStruct
		dc, _, _ := pBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		// Drawn off screen first, so a redraw (hovering) does not flicker.
		mem, _, _ := pCreateCompatibleDC.Call(dc)
		bmp, _, _ := pCreateCompatibleBitmap.Call(dc, uintptr(c.w), uintptr(c.h))
		old, _, _ := pSelectObject.Call(mem, bmp)
		c.paint(mem)
		pBitBlt.Call(dc, 0, 0, uintptr(c.w), uintptr(c.h), mem, 0, 0, srcCopy)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mem)
		pEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		return 0
	case wmSetCursor:
		shape := uintptr(idcArrow)
		if pt := cursor(hwnd); c.close.has(pt) || c.linkAt(pt) >= 0 {
			shape = idcHand
		}
		h, _, _ := pLoadCursor.Call(0, shape)
		pSetCursor.Call(h)
		return 1
	case wmMouseMove:
		c.hover(hwnd, cursor(hwnd), true)
		t := trackMouseEvent{Flags: tmeLeave, Hwnd: hwnd}
		t.Size = uint32(unsafe.Sizeof(t))
		pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
		return 0
	case wmMouseLeave:
		c.hover(hwnd, point{}, false)
		return 0
	case wmLButtonUp:
		pt := cursor(hwnd)
		if c.close.has(pt) {
			c.dismiss()
		} else if i := c.linkAt(pt); i >= 0 {
			// The reminder stays, as it may have more links, unless the link says to close it.
			part := c.s.parts[i]
			go func() {
				if err := browser.Open(part.Link); err != nil {
					fmt.Fprintf(os.Stderr, "opening %s: %v\n", part.Link, err)
				}
			}()
			if part.Close {
				c.dismiss()
			}
		}
		return 0
	case wmDestroy:
		cardsMu.Lock()
		delete(cards, hwnd)
		cardsMu.Unlock()
		c.free()
		if c.s.open--; c.s.open == 0 {
			pPostQuitMessage.Call(0)
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
	return r
}

// playChime plays the chime without waiting for it. The sound is kept (chime), as PlaySound reads
// it while it plays.
func playChime() {
	if pPlaySound.Find() != nil {
		return
	}
	pPlaySound.Call(uintptr(unsafe.Pointer(&chime()[0])), 0, sndMemory|sndAsync|sndNoDefault)
}

// stopChime cuts the chime playing short.
func stopChime() {
	if pPlaySound.Find() == nil {
		pPlaySound.Call(0, 0, 0)
	}
}
