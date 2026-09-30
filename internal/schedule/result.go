package schedule

import (
	"regexp"
	"strconv"
	"strings"
)

var scorePattern = regexp.MustCompile(`^(\d+)\s*-\s*(\d+)$`)

type GameOutcome struct {
	Outcome       string
	HomeScore     int
	AwayScore     int
	PlayerScore   int
	OpponentScore int
	Raw           string
	Parsed        bool
}

// ParseGameResult reads a result from the player team's perspective, taking
// that team as the home side unless its name differs from the home team's.
func ParseGameResult(result, playerTeamName, homeTeamName string) GameOutcome {
	isHomeTeam := true
	playerTeamName = strings.TrimSpace(playerTeamName)
	homeTeamName = strings.TrimSpace(homeTeamName)
	if playerTeamName != "" && homeTeamName != "" {
		isHomeTeam = strings.EqualFold(playerTeamName, homeTeamName)
	}
	return ParseGameResultForSide(result, isHomeTeam)
}

// ParseGameResultForSide reads a result from the perspective of the home side
// when isHomeTeam is true, or the away side otherwise.
func ParseGameResultForSide(result string, isHomeTeam bool) GameOutcome {
	raw := strings.TrimSpace(result)
	if raw == "" {
		return GameOutcome{}
	}

	if strings.EqualFold(raw, gameStatusCanceled) {
		return GameOutcome{Outcome: gameOutcomeCanceled, Raw: raw, Parsed: true}
	}
	if strings.EqualFold(raw, gameStatusFinal) {
		return GameOutcome{Outcome: gameOutcomeFinal, Raw: raw, Parsed: true}
	}

	matches := scorePattern.FindStringSubmatch(raw)
	if len(matches) != 3 {
		return GameOutcome{Outcome: raw, Raw: raw, Parsed: false}
	}

	homeScore, err := strconv.Atoi(matches[1])
	if err != nil {
		return GameOutcome{Outcome: raw, Raw: raw, Parsed: false}
	}
	awayScore, err := strconv.Atoi(matches[2])
	if err != nil {
		return GameOutcome{Outcome: raw, Raw: raw, Parsed: false}
	}

	playerScore := homeScore
	opponentScore := awayScore
	if !isHomeTeam {
		playerScore = awayScore
		opponentScore = homeScore
	}

	outcome := gameOutcomeDraw
	if playerScore > opponentScore {
		outcome = gameOutcomeWin
	} else if playerScore < opponentScore {
		outcome = gameOutcomeLoss
	}

	return GameOutcome{
		Outcome:       outcome,
		HomeScore:     homeScore,
		AwayScore:     awayScore,
		PlayerScore:   playerScore,
		OpponentScore: opponentScore,
		Raw:           raw,
		Parsed:        true,
	}
}

func FormatResultLine(outcome GameOutcome) string {
	if !outcome.Parsed {
		return outcome.Raw
	}

	switch outcome.Outcome {
	case gameOutcomeWin, gameOutcomeLoss, gameOutcomeDraw:
		return outcome.Outcome + " (" + strconv.Itoa(outcome.PlayerScore) + "-" + strconv.Itoa(outcome.OpponentScore) + ")"
	case gameOutcomeCanceled:
		return gameOutcomeCanceled
	case gameOutcomeFinal:
		return gameOutcomeFinal
	default:
		return outcome.Raw
	}
}
