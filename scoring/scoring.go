package scoring

// PlayerStats holds raw match performance metrics
type PlayerStats struct {
	Kills       int
	Deaths      int
	Assists     int
	CS          int
	TripleKills int
	QuadraKills int
	PentaKills  int
	Win         bool
}

// CalculateFantasyPoints computes total points based on standard fantasy rules
func CalculateFantasyPoints(stats PlayerStats) float64 {
	points := 0.0

	// Core Performance
	points += float64(stats.Kills) * 3.0
	points -= float64(stats.Deaths) * 1.0
	points += float64(stats.Assists) * 2.0
	points += float64(stats.CS) * 0.02

	// Multi-kill bonuses
	points += float64(stats.TripleKills) * 2.0
	points += float64(stats.QuadraKills) * 5.0
	points += float64(stats.PentaKills) * 10.0

	// Team Win Bonus
	if stats.Win {
		points += 5.0
	}

	return points
}
