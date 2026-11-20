package asr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

func Format(a Audio) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(a.Name), "."))
	if ext == "wave" {
		return "wav"
	}
	if ext == "oga" {
		return "ogg"
	}
	return ext
}

// WAV metadata lets callers enforce limits without a codec or native dependency.
func wavInfo(data []byte) (rate, channels, bits int, duration int64, err error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return 0, 0, 0, 0, errors.New("invalid WAV header")
	}
	var bytesPerSecond uint32
	var length uint32
	for offset := 12; offset+8 <= len(data); {
		size := int64(binary.LittleEndian.Uint32(data[offset+4:]))
		end := int64(offset) + 8 + size
		if end > int64(len(data)) {
			return 0, 0, 0, 0, errors.New("truncated WAV chunk")
		}
		chunk := data[offset+8 : int(end)]
		switch string(data[offset : offset+4]) {
		case "fmt ":
			if len(chunk) < 16 {
				return 0, 0, 0, 0, errors.New("invalid WAV format")
			}
			channels = int(binary.LittleEndian.Uint16(chunk[2:]))
			rate = int(binary.LittleEndian.Uint32(chunk[4:]))
			bytesPerSecond = binary.LittleEndian.Uint32(chunk[8:])
			bits = int(binary.LittleEndian.Uint16(chunk[14:]))
			if binary.LittleEndian.Uint16(chunk) != 1 {
				bits = 0
			}
		case "data":
			length = uint32(size)
		}
		offset = int(end + (size & 1))
	}
	if rate == 0 || bytesPerSecond == 0 || length == 0 {
		return 0, 0, 0, 0, errors.New("WAV has no audio or format data")
	}
	return rate, channels, bits, int64(length) * 1000 / int64(bytesPerSecond), nil
}
func (d Definition) ValidateAudio(a Audio) error {
	if len(a.Data) == 0 || len(a.Data) > d.MaxBytes {
		return fmt.Errorf("%s requires a nonempty audio file of at most %d bytes", d.Name, d.MaxBytes)
	}
	format := Format(a)
	if !slices.Contains(d.Formats, format) {
		return fmt.Errorf("%s supports %s; received %q. Audio conversion is not performed automatically", d.Name, strings.Join(d.Formats, ", "), format)
	}
	if format == "wav" {
		rate, channels, bits, duration, err := wavInfo(a.Data)
		if err != nil {
			return err
		}
		if d.MaxSeconds > 0 && duration > int64(d.MaxSeconds)*1000 {
			return fmt.Errorf("%s audio duration exceeds %d seconds", d.Name, d.MaxSeconds)
		}
		if d.ID == DeepSeek && (bits != 16 || channels < 1 || channels > 2 || !slices.Contains([]int{8000, 16000, 22050, 24000, 32000, 44100, 48000}, rate)) {
			return errors.New("DeepSeek requires 16-bit PCM WAV, mono/stereo, at a supported sample rate")
		}
	}
	if d.ID == DeepSeek && format == "ogg" && !strings.Contains(string(a.Data[:min(len(a.Data), 512)]), "OpusHead") {
		return errors.New("DeepSeek requires Ogg Opus audio")
	}
	return nil
}
