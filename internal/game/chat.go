package game

import (
	"strings"
	"unicode"
)

type ChatLine struct {
	PlayerID string `json:"playerId" bson:"playerId"`
	Name     string `json:"name" bson:"name"`
	Color    string `json:"color,omitempty" bson:"color,omitempty"`
	Text     string `json:"text" bson:"text"`
}

func (s *State) Say(playerID, text string) error {
	i := s.playerIndex(playerID)
	if i < 0 {
		return ErrNotInGame
	}
	text = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, text))
	runes := []rune(text)
	if len(runes) == 0 {
		return ErrBadSettings
	}
	if len(runes) > 200 {
		text = string(runes[:200])
	}
	s.Chat = append(s.Chat, ChatLine{
		PlayerID: playerID,
		Name:     s.Players[i].Name,
		Color:    s.Players[i].Color,
		Text:     text,
	})
	if len(s.Chat) > 60 {
		s.Chat = s.Chat[len(s.Chat)-60:]
	}
	return nil
}
