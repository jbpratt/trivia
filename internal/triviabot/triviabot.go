package triviabot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/jbpratt/bots/internal/bot"
	"github.com/jbpratt/bots/internal/trivia"
	"go.uber.org/zap"
)

type TriviaBot struct {
	logger          *zap.SugaredLogger
	bot             *bot.Bot
	quiz            *trivia.Quiz
	leaderboard     *trivia.Leaderboard
	lastQuizEndedAt int64
}

var (
	failureEmotes = []string{"TANTIES", "JimFace", "DuckerZ"}
	successEmotes = []string{""}
)

func New(
	logger *zap.SugaredLogger,
	url, jwt, path string,
	duration time.Duration,
) (*TriviaBot, error) {

	quiz, err := trivia.NewDefaultQuiz(logger)
	if err != nil {
		return nil, fmt.Errorf("error creating trivia: %w", err)
	}

	bot, err := bot.New(logger, url, jwt)
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %w", err)
	}

	var lboard *trivia.Leaderboard
	lboard, err = trivia.NewLeaderboard(logger, path)
	if err != nil {
		return nil, fmt.Errorf("failed to init leaderboard: %w", err)
	}

	t := &TriviaBot{logger, bot, quiz, lboard, 0}
	bot.OnMessage(t.onMsg)
	bot.OnPrivMessage(t.onPrivMsg)

	return t, nil
}

func (t *TriviaBot) Run() error {
	return t.bot.Run()
}

func (t *TriviaBot) onMsg(ctx context.Context, msg *bot.Msg) error {
	// TODO: can we use a FlagSet for parsing commands?

	if !strings.HasPrefix(msg.Data, "trivia") && !strings.HasPrefix(msg.Data, "!trivia") {
		return nil
	}

	if strings.Contains(msg.Data, "help") {
		if err := t.bot.Send(
			"Start a new round with `trivia start`. PM the number beside the answer `/w trivia 2`",
		); err != nil {
			return fmt.Errorf("failed to send help msg: %w", err)
		}
	}

	// TODO: when someone answer in public chat, send PM instructing user how to
	// properly answer
	// if t.quiz.InProgress {
	// }

	if strings.Contains(msg.Data, "start") || strings.Contains(msg.Data, "new") {
		if t.quiz.InProgress {
			if err := t.bot.Send(fmt.Sprintf("a quiz is already in progress %s", "")); err != nil {
				return fmt.Errorf("failed to send quiz in progress msg: %w", err)
			}
			return nil
		}

		fiveMinAgo := time.Now().Add(-5 * time.Minute).UnixNano()
		if fiveMinAgo < t.lastQuizEndedAt && !strings.Contains(msg.Data, "force") {
			// TODO: send how much longer we are on cooldown for
			if err := t.bot.Send("on cooldown PepoSleep"); err != nil {
				return fmt.Errorf("failed to send quiz in progress msg: %w", err)
			}
			return nil
		}

		// TODO: allow for providing quiz size
		var err error
		if !t.quiz.InProgress {
			t.quiz, err = trivia.NewDefaultQuiz(t.logger)
			if err != nil {
				return fmt.Errorf("failed to create a new quiz: %w", err)
			}
		}

		go func() {
			if err = t.runQuiz(ctx); err != nil {
				t.logger.Fatalf("failed while running the quiz: %v", err)
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
			if err = t.bot.SendPriv(
				"Invalid answer, PM the number of the answer",
				msg.User,
			); err != nil {
				return nil
			}
		}

		if !t.quiz.CurrentRound.NewParticipant(msg.User, answer-1, msg.Time) {
			if err = t.bot.SendPriv("you have already submitted an answer!", msg.User); err != nil {
				return nil
			}
		}
	}

	return nil
}

func (t *TriviaBot) runQuiz(ctx context.Context) error {
	if t.quiz.InProgress {
		return errors.New("quiz is already in progress")
	}

	t.logger.Info("starting quiz")
	round, err := t.quiz.StartRound(t.onRoundCompletion)
	if err != nil {
		return fmt.Errorf("failed to start the round: %w", err)
	}

	output := "Quiz starting soon! PM the number beside the answer. First 3 correct answers are awarded points."
	if err = t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send starting message: %w", err)
	}

	time.Sleep(10 * time.Second)

	if err = t.runRound(ctx, round); err != nil {
		return fmt.Errorf("error running round: %w", err)
	}

	// continue running rounds
	for round.NextRound != nil {
		t.logger.Infof("running next round %d", round.Num)
		round, err = t.quiz.StartRound(t.onRoundCompletion)
		if err != nil {
			return fmt.Errorf("failed to start the round: %w", err)
		}

		if err = t.runRound(ctx, round); err != nil {
			return fmt.Errorf("error running round: %w", err)
		}
	}

	time.Sleep(5 * time.Second)

	output = "Quiz complete! Winners: "
	if len(t.quiz.Scoreboard) == 0 {
		output += "No one! DuckerZ"
	} else {
		ss := t.quiz.SortedScore()
		limit := 3
		for name, points := range ss {
			if limit == 0 {
				break
			}
			output += fmt.Sprintf("`%s +%d point(s)` ", name, points)
			limit--
		}

		if err = t.leaderboard.Update(ss); err != nil {
			return fmt.Errorf("failed to update leaderboard: %w", err)
		}
	}

	return t.bot.Send(output)
}

func (t *TriviaBot) runRound(ctx context.Context, round *trivia.Round) error {
	if round.NextRound != nil {
		defer func() {
			t.logger.Info("sleeping for 25 seconds until next round")
			time.Sleep(25 * time.Second)
		}()
	}

	leading := fmt.Sprintf("Round %d", round.Num)
	if round.NextRound == nil {
		leading = "Final round"
	}

	output := fmt.Sprintf("%s: %q (%s). `%s` ", leading, round.Category, round.Difficulty, round.Question)

	// answers have already been shuffled
	for idx, ans := range round.Answers {
		output += fmt.Sprintf(" `%d) %s`", idx+1, ans.Value)
	}

	t.logger.Infow("running round and waiting for completion", "output", output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round start msgs: %w", err)
	}

	for {
		if !t.quiz.InProgress {
			t.logger.Info("round is no longer in progress.. breaking")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

func (t *TriviaBot) onRoundCompletion(correct string, score []*trivia.Participant) error {
	output := fmt.Sprintf("Round complete! The correct answer is %s.", correct)
	if len(score) == 0 {
		return t.bot.Send(output + " No one answered correctly DuckerZ")
	}

	for i := 0; i < len(score) && i <= 2; i++ {
		output += fmt.Sprintf(" %s %s", humanize.Ordinal(i+1), score[i].Name)

		if len(score) >= 2 && i > 0 {
			timeDiff := time.Unix(score[i].TimeIn-score[i-1].TimeIn, 0)
			output += fmt.Sprintf(" +%s", humanize.Time(timeDiff))
		}
	}

	t.logger.Info(output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round completion msg: %w", err)
	}

	t.lastQuizEndedAt = time.Now().UnixNano()

	return nil
}
