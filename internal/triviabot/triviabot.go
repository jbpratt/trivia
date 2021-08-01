package triviabot

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jbpratt/bots/internal/bot"
	"github.com/jbpratt/bots/internal/trivia"
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

	var lboard *trivia.Leaderboard
	lboard, err = trivia.NewLeaderboard(logger, path)
	if err != nil {
		return nil, fmt.Errorf("failed to init leaderboard: %v", err)
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
			return fmt.Errorf("failed to send help msg: %v", err)
		}
	}

	if strings.Contains(msg.Data, "start") || strings.Contains(msg.Data, "new") {
		if t.quiz.InProgress {
			if err := t.bot.Send("a quiz is already in progress"); err != nil {
				return fmt.Errorf("failed to send quiz in progress msg: %v", err)
			}
		}

		//fiveMinAgo := time.Now().Add(-5 * time.Minute).UnixNano()
		//if t.lastQuizTime < fiveMinAgo && !strings.Contains(msg.Data, "force") {
		// send err
		// "on cooldown for XmXs..."
		//}

		// TODO: allow for providing quiz size
		var err error
		if !t.quiz.InProgress {
			t.quiz, err = trivia.NewDefaultQuiz(t.logger)
			if err != nil {
				return fmt.Errorf("failed to create a new quiz: %v", err)
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

	if err = t.bot.Send("Quiz starting soon! PM the number beside the answer. First 3 in get awarded points."); err != nil {
		return fmt.Errorf("failed to send starting message: %w", err)
	}

	time.Sleep(10 * time.Second)

	if err = t.runRound(ctx, round); err != nil {
		return fmt.Errorf("error running round: %v", err)
	}

	// continue running rounds
	for round.NextRound != nil {
		t.logger.Infof("running next round %d", round.Num)
		round, err = t.quiz.StartRound(t.onRoundCompletion)
		if err != nil {
			return fmt.Errorf("failed to start the round: %v", err)
		}

		if err = t.runRound(ctx, round); err != nil {
			return fmt.Errorf("error running round: %v", err)
		}

		// break early before sleeping
		if round.NextRound == nil {
			break
		}

		t.logger.Info("sleeping for 10 seconds until next round")
		time.Sleep(10 * time.Second)
	}

	output := "Quiz complete! Winners: "
	if len(t.quiz.Score) == 0 {
		output += "No one! DuckerZ"
	} else {
		// sort winners by points for top 3
		sort.Slice(t.quiz.Score, func(i, j int) bool {
			return t.quiz.Score[i].Points > t.quiz.Score[j].Points
		})

		limit := 3
		for _, score := range t.quiz.Score {
			if limit == 0 {
				break
			}
			output += fmt.Sprintf("%s - %d point(s) ", score.Name, score.Points)
			limit--
		}

		// update leaderboard at the end of the quiz with all users' points
		data := map[string]int{}
		for _, score := range t.quiz.Score {
			data[score.Name] = score.Points
		}

		if err = t.leaderboard.Update(data); err != nil {
			return fmt.Errorf("failed to update leaderboard: %v", err)
		}
	}

	return t.bot.Send(output)
}

func (t *TriviaBot) runRound(ctx context.Context, round *trivia.Round) error {
	leading := fmt.Sprintf("Round %d", round.Num)
	if round.NextRound == nil {
		leading = "Final round"
	}

	output := fmt.Sprintf("%s, PM the number. %q (%s). Question: `%s` Answers:", leading, round.Category, round.Difficulty, round.Question)

	// answers have already been shuffled
	for idx, ans := range round.Answers {
		output += fmt.Sprintf(" `%d) %s`", idx+1, ans.Value)
	}

	t.logger.Infow("running round and waiting for completion", "output", output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round start msgs: %v", err)
	}

	for {
		if !t.quiz.InProgress {
			t.logger.Info("round is no longer in progress.. breaking")
			break
		}
	}

	return nil
}

func (t *TriviaBot) onRoundCompletion(correct string, score []*trivia.Participant) error {
	// Delay the start of the next round
	defer time.Sleep(10 * time.Second)

	output := fmt.Sprintf("Round complete! Answer: %s", correct)
	if len(score) == 0 {
		return t.bot.Send(output + " No one answered correctly DuckerZ")
	}

	output += " Winners:"
	for i := 0; i < len(score) && i <= 2; i++ {
		output += fmt.Sprintf(" %d: %s", i+1, score[i].Name)

		//	timeDiff := score[i].TimeIn - score[i-1].TimeIn
		//	output += fmt.Sprintf(" +%d", timeDiff)
	}

	t.logger.Info(output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round completion msg: %v", err)
	}

	return nil
}
