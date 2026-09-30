package reminder

import (
	"encoding/binary"
	"math"
	"sync"
	"time"
)

// chime is the sound a reminder plays: two soft bell notes, a fifth apart, as a WAV file (16-bit
// mono PCM), made once.
var chime = sync.OnceValue(func() []byte {
	const (
		rate   = 44100
		length = 1.8 // seconds
		volume = 0.28
	)
	notes := []struct{ freq, start float64 }{{659.25, 0}, {987.77, 0.16}} // E5, B5
	samples := make([]float64, int(rate*length))
	for _, n := range notes {
		for i := int(n.start * rate); i < len(samples); i++ {
			t := float64(i)/rate - n.start
			// A quick rise (no click), then a bell's long fade; faint overtones soften it.
			env := math.Min(1, t/0.006) * math.Exp(-t*3)
			w := 2 * math.Pi * n.freq * t
			samples[i] += env * (math.Sin(w) + 0.2*math.Sin(2*w)*math.Exp(-t*4) + 0.06*math.Sin(3*w)*math.Exp(-t*6))
		}
	}
	peak := 0.0
	for _, s := range samples {
		peak = math.Max(peak, math.Abs(s))
	}
	data := make([]byte, 2*len(samples))
	for i, s := range samples {
		// Fade the last 50 ms out, so it ends in silence.
		if left := len(samples) - i; left < rate/20 {
			s *= float64(left) / (rate / 20)
		}
		binary.LittleEndian.PutUint16(data[2*i:], uint16(int16(s/peak*volume*math.MaxInt16)))
	}
	wav := make([]byte, 44, 44+len(data))
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(36+len(data)))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)     // fmt chunk size
	binary.LittleEndian.PutUint16(wav[20:], 1)      // PCM
	binary.LittleEndian.PutUint16(wav[22:], 1)      // mono
	binary.LittleEndian.PutUint32(wav[24:], rate)   // samples per second
	binary.LittleEndian.PutUint32(wav[28:], rate*2) // bytes per second
	binary.LittleEndian.PutUint16(wav[32:], 2)      // bytes per sample
	binary.LittleEndian.PutUint16(wav[34:], 16)     // bits per sample
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], uint32(len(data)))
	return append(wav, data...)
})

// announce plays the chime for a reminder shown, without waiting: once, or for an urgent one twice
// at once and twice again every urgentEvery, until stop closes (quiet then cuts the chime short)
// or urgentFor passes.
func announce(urgent bool, stop <-chan struct{}, play, quiet func()) {
	if !urgent {
		play()
		return
	}
	go func() {
		end := time.NewTimer(urgentFor)
		defer end.Stop()
		tick := time.NewTicker(urgentEvery)
		defer tick.Stop()
		wait := func(c <-chan time.Time) bool {
			select {
			case <-stop:
				quiet()
				return false
			case <-end.C:
				return false
			case <-c:
				return true
			}
		}
		for {
			play()
			if !wait(time.After(chimeGap)) {
				return
			}
			play()
			if !wait(tick.C) {
				return
			}
		}
	}()
}
