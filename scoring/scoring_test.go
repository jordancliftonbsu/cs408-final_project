package scoring

import "testing"

func TestCalculateFantasyPoints(t *testing.T) {
	tests := []struct {
		name     string
		stats    PlayerStats
		expected float64
	}{
		{
			name: "Standard Game Stats (Loss)",
			stats: PlayerStats{
				Kills:   5,
				Deaths:  2,
				Assists: 10,
				CS:      250,
				Win:     false,
			},
			// (5 * 3.0) - (2 * 1.0) + (10 * 2.0) + (250 * 0.02)
			// 15 - 2 + 20 + 5.0 = 38.0
			expected: 38.0,
		},
		{
			name: "Standard Game Stats (Win Bonus Included)",
			stats: PlayerStats{
				Kills:   5,
				Deaths:  2,
				Assists: 10,
				CS:      250,
				Win:     true,
			},
			// 38.0 + 5.0 (Win) = 43.0
			expected: 43.0,
		},
		{
			name: "Game with Triple Kill Bonus and Win",
			stats: PlayerStats{
				Kills:       6,
				Deaths:      1,
				Assists:     4,
				CS:          200,
				TripleKills: 1,
				Win:         true,
			},
			// (6 * 3) - 1 + (4 * 2) + (200 * 0.02) + 2 (Triple) + 5 (Win)
			// 18 - 1 + 8 + 4.0 + 2.0 + 5.0 = 36.0
			expected: 36.0,
		},
		{
			name: "Game with Quadra Kill Bonus and Loss",
			stats: PlayerStats{
				Kills:       8,
				Deaths:      3,
				Assists:     5,
				CS:          220,
				QuadraKills: 1,
				Win:         false,
			},
			// (8 * 3) - 3 + (5 * 2) + (220 * 0.02) + 5 (Quadra)
			// 24 - 3 + 10 + 4.4 + 5.0 = 40.4
			expected: 40.4,
		},
		{
			name: "Game with PentaKill Bonus and Win",
			stats: PlayerStats{
				Kills:      10,
				Deaths:     1,
				Assists:    5,
				CS:         300,
				PentaKills: 1,
				Win:        true,
			},
			// (10 * 3) - 1 + (5 * 2) + (300 * 0.02) + 10 (Penta) + 5 (Win)
			// 30 - 1 + 10 + 6.0 + 10.0 + 5.0 = 60.0
			expected: 60.0,
		},

		// --- EDGE CASES ---

		{
			name:     "Edge Case: All Zero Stats",
			stats:    PlayerStats{},
			expected: 0.0,
		},
		{
			name: "Edge Case: High Deaths Resulting in Negative Score (Loss)",
			stats: PlayerStats{
				Kills:   0,
				Deaths:  8,
				Assists: 1,
				CS:      50,
				Win:     false,
			},
			// (0 * 3.0) - (8 * 1.0) + (1 * 2.0) + (50 * 0.02)
			// 0 - 8 + 2 + 1.0 = -5.0
			expected: -5.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateFantasyPoints(tt.stats)
			if got != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, got)
			}
		})
	}
}
