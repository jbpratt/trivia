package triviabot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jbpratt/trivia/internal/bot"
	"github.com/jbpratt/trivia/internal/trivia"
	"go.uber.org/zap"
)

type TriviaBot struct {
	logger       *zap.SugaredLogger
	bot          *bot.Bot
	quiz         *trivia.Quiz
	leaderboard  *trivia.Leaderboard
	lastQuizTime int64
}

func New(
	logger *zap.SugaredLogger,
	wsURL, wsJWT, path string,
	quizSize int,
	duration time.Duration,
) (*TriviaBot, error) {

	quiz, err := trivia.NewDefaultQuiz(logger)
	if err != nil {
		return nil, fmt.Errorf("error creating trivia: %v", err)
	}

	bot, err := bot.New(logger, wsURL, wsJWT)
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %v", err)
	}

	//var lboard *trivia.Leaderboard
	//lboard, err = trivia.NewLeaderboard(logger, path)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to init leaderboard: %v", err)
	//}

	// t := &TriviaBot{logger, bot, quiz, lboard, 0}
	t := &TriviaBot{logger, bot, quiz, nil, 0}

	bot.OnMessage(t.onMsg)
	bot.OnPrivMessage(t.onPrivMsg)

	return t, nil
}

func (t *TriviaBot) Run() error {
	return t.bot.Run()
}

func (t *TriviaBot) onMsg(ctx context.Context, msg *bot.Msg) error {
	if !strings.HasPrefix(msg.Data, "trivia") && !strings.HasPrefix(msg.Data, "!trivia") {
		return nil
	}

	if strings.Contains(msg.Data, "help") {
		if err := t.bot.Send("PM the number beside the answer `/w trivia 2`"); err != nil {
			return fmt.Errorf("failed to send help msg: %v", err)
		}
	}

	if strings.Contains(msg.Data, "start") || strings.Contains(msg.Data, "new") {
		if t.quiz.InProgress {
			if err := t.bot.Send("a quiz is already in progress"); err != nil {
				return fmt.Errorf("failed to send quiz in progress msg: %v", err)
			}
		}

		// lastQuiz := time.Unix(t.lastQuiz, 0)
		// if t.lastQuiz was within 5 minutes and msg.Data != contain(force)

		// allow for providing quiz size
		var err error
		if t.quiz.FirstRound.Complete {
			t.quiz, err = trivia.NewDefaultQuiz(t.logger)
			if err != nil {
				return fmt.Errorf("failed to create a new quiz: %v", err)
			}
		}

		go func() {
			if err = t.runQuiz(ctx); err != nil {
				t.logger.Fatal("failed to run the quiz: %v", err)
			}
		}()
	}

	return nil
}

func (t *TriviaBot) onPrivMsg(ctx context.Context, msg *bot.Msg) error {
	t.logger.Debugw("private message received", "user", msg.User, "msg", msg.Data)
	if t.quiz.InProgress {
		answer, err := strconv.Atoi(msg.Data)
		if err != nil {
			if err := t.bot.SendPriv(
				"Invalid answer, PM the number of the answer",
				msg.User,
			); err != nil {
				return nil
			}
		}

		if !t.quiz.CurrentRound.NewParticipant(msg.User, answer-1, msg.Time) {
			if err := t.bot.SendPriv("you have already submitted an answer!", msg.User); err != nil {
				return nil
			}
		}
	}

	return nil
}

func (t *TriviaBot) runQuiz(ctx context.Context) error {
	t.logger.Info("starting quiz")
	round, err := t.quiz.StartRound(t.onRoundCompletion)
	if err != nil {
		return fmt.Errorf("failed to start the round: %v", err)
	}

	if err = t.runRound(ctx, round); err != nil {
		return fmt.Errorf("error running round: %v", err)
	}

	t.logger.Info("running next round")
	// continue running rounds
	for round.NextRound != nil {
		round, err = t.quiz.StartRound(t.onRoundCompletion)
		if err != nil {
			return fmt.Errorf("failed to start the round: %v", err)
		}

		if err = t.runRound(ctx, round); err != nil {
			return fmt.Errorf("error running round: %v", err)
		}

		t.logger.Info("sleeping for 10 seconds until next round")
		time.Sleep(10 * time.Second)
		t.logger.Info("running next round")
	}

	return nil
}

func (t *TriviaBot) runRound(ctx context.Context, round *trivia.Round) error {
	output := fmt.Sprintf("New round starting, PM the number. `%s | %s`. Question: `%s`", round.Category, round.Difficulty, round.Question)
	// answers have already been shuffled
	for idx, ans := range round.Answers {
		output += fmt.Sprintf(" `%d: %s`", idx+1, ans.Value)
	}

	t.logger.Infow("running round", "output", output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round start msgs: %v", err)
	}

	t.logger.Info("waiting for quiz completion")
	for {
		if !t.quiz.InProgress {
			t.logger.Infow("quiz is no longer in progress.. breaking", "in_progress", t.quiz.InProgress)
			break
		}
	}

	return nil
}

func (t *TriviaBot) onRoundCompletion(correct string, score map[int]string) error {
	output := fmt.Sprintf("Round complete! The correct answer is: %s", correct)
	defer func() {
		t.logger.Info(output)
		time.Sleep(10 * time.Second)
	}()

	if len(score) == 0 {
		output += " No one answered correctly."
		return t.bot.Send(output)
	}

	spew.Dump(score)
	output += " The winners are:"
	for i := 0; i < len(score) || i == 2; i++ {
		output += fmt.Sprintf(" `%d: %s`", i+1, score[i])
	}

	// update leaderboard

	return t.bot.Send(output)
}
