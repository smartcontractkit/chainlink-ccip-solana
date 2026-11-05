package loadtest

import (
	"github.com/smartcontractkit/chainlink-ccv/build/devenv/tests/e2e/load"
)

type messageProfileMixEntry struct {
	Name        string  `json:"name"`
	Weight      int     `json:"weight"`
	ExpectedPct float64 `json:"expected_pct"`
	DataBytes   int     `json:"data_bytes"`
}

// expectedProfileMix derives configured payload mix percentages from CCV profile ratios.
func expectedProfileMix(profiles []load.MessageProfileConfig, ratios []load.MessageTestRatio) []messageProfileMixEntry {
	if len(ratios) == 0 {
		return nil
	}

	byName := make(map[string]load.MessageProfileConfig, len(profiles))
	for _, p := range profiles {
		byName[p.Name] = p
	}

	totalWeight := 0
	for _, r := range ratios {
		totalWeight += r.Ratio
	}
	if totalWeight == 0 {
		return nil
	}

	entries := make([]messageProfileMixEntry, 0, len(ratios))
	for _, r := range ratios {
		p, ok := byName[r.MessageProfile]
		if !ok {
			continue
		}
		dataBytes := 0
		if p.HasData {
			dataBytes = load.MessageDataSizeBytes(p, 1000)
		}
		entries = append(entries, messageProfileMixEntry{
			Name:        p.Name,
			Weight:      r.Ratio,
			ExpectedPct: float64(r.Ratio) / float64(totalWeight) * 100,
			DataBytes:   dataBytes,
		})
	}
	return entries
}
