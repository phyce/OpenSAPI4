package synthesizer

// MuteVolume is the lowest volume that doesn't mute.
const MuteVolume = 80

// attenuationThresholds[k] is the lowest volume attenuated by k dB.
var attenuationThresholds = [...]uint16{52057, 41350, 32846, 26090, 20724, 16462, 13076, 10387, 8251, 6554, 5206, 4135, 3285, 2609, 2073}

// Attenuation returns a volume's attenuation in dB, 0 to 15, or false for a volume that mutes (synthesizer.md
// §5.4).
func Attenuation(volume uint16) (int32, bool) {
	if volume < MuteVolume {
		return 0, false
	}
	for attenuation, threshold := range attenuationThresholds {
		if volume >= threshold {
			return int32(attenuation), true
		}
	}
	return int32(len(attenuationThresholds)), true
}

// AppendPCM appends samples as 16-bit little-endian PCM, or with 8 bits per sample as unsigned bytes: the high
// byte plus 128.
func AppendPCM(pcm []byte, samples []int16, bitsPerSample int) []byte {
	for _, sample := range samples {
		if bitsPerSample == 8 {
			pcm = append(pcm, byte(sample>>8+128))
		} else {
			pcm = append(pcm, byte(sample), byte(sample>>8))
		}
	}
	return pcm
}
