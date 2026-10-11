package whistler

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestFormatOffers(t *testing.T) {
	data := loadData(t)
	expected := map[string][]Format{
		"Mary":                 {{22050, 16}, {22050, 8}, {8000, 16}, {8000, 8}, {16000, 16}, {16000, 8}},
		"Mike (for Telephone)": {{8000, 16}, {8000, 8}, {11025, 16}, {11025, 8}},
	}
	for name, offers := range expected {
		mode, _ := data.Mode(name)
		var offered []Format
		_, err := data.NewEngineAccepting(mode, func(format Format) error {
			offered = append(offered, format)
			return fmt.Errorf("refused %d", len(offered))
		})
		if !slices.Equal(offered, offers) || err == nil || err.Error() != fmt.Sprintf("refused %d", len(offers)) {
			t.Errorf("%s: offered %v and failed with %v", name, offered, err)
		}
		for index, format := range offers {
			offered = nil
			engine, err := data.SelectAccepting(mode.ModeID, func(offer Format) error {
				offered = append(offered, offer)
				if offer != format {
					return errRefused
				}
				return nil
			})
			if err != nil || engine.Format() != format || len(offered) != index+1 {
				t.Errorf("%s accepting %v: %v after %d offers", name, format, err, len(offered))
			}
		}
	}
	if _, err := data.SelectAccepting("{00000000-0000-0000-0000-000000000001}", func(Format) error { return nil }); !errors.Is(err, ErrInvalidMode) {
		t.Errorf("an unknown mode gave %v", err)
	}
	engine, err := data.Select(NullModeID)
	if err != nil || engine.Format() != (Format{engine.voice.SampleRate, 16}) {
		t.Errorf("the first hidden mode: %v, %+v", err, engine.Format())
	}
}
