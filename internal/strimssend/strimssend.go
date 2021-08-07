package strimssend

import (
	"fmt"

	"github.com/jbpratt/bots/internal/bot"
	"go.uber.org/zap"
)

type StrimsSend struct {
	logger *zap.SugaredLogger
	bot    *bot.Bot
}

func New(logger *zap.SugaredLogger, url, jwt string) (*StrimsSend, error) {
	bot, err := bot.New(logger, url, jwt)
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %w", err)
	}
	return &StrimsSend{logger, bot}, nil
}

func (s *StrimsSend) Send(data string) error {
	return s.bot.Send(data)
}
