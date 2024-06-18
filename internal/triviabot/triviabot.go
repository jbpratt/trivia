package triviabot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/dustin/go-humanize/english"
	"github.com/jbpratt/bots/internal/bot"
	"github.com/jbpratt/bots/internal/trivia"
	"github.com/jbpratt/bots/internal/trivia/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"go.uber.org/zap"
)

type TriviaBot struct {
	logger                *zap.SugaredLogger
	bot                   *bot.Bot
	source                trivia.Source
	quiz                  *trivia.Quiz
	leaderboard           *trivia.Leaderboard
	lastQuizEndedAt       time.Time
	leaderboardOutputPath string
	leaderboardIngress    string
}

func New(
	logger *zap.SugaredLogger,
	url, jwt, dbPath, lboardOutputPath, lboardIngress string,
) (*TriviaBot, error) {
	filters := []bot.MsgTypeFilter{
		bot.JoinFilter,
		bot.QuitFilter,
		bot.ViewerStateFilter,
		bot.NamesFilter,
		bot.PrivMsgSentFilter,
	}

	bot, err := bot.New(logger, bot.WebSocketDialer, url, jwt, true, filters...)
	if err != nil {
		return nil, fmt.Errorf("error creating bot: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open DB(%s): %w", dbPath, err)
	}

	boil.SetDB(db)

	source, err := trivia.NewDefaultDBSource(db)
	if err != nil {
		return nil, fmt.Errorf("failed to create DB source: %w", err)
	}

	var lboard *trivia.Leaderboard
	lboard, err = trivia.NewLeaderboard(logger, db)
	if err != nil {
		return nil, fmt.Errorf("failed to init leaderboard: %w", err)
	}

	t := &TriviaBot{
		logger:                logger,
		bot:                   bot,
		source:                source,
		leaderboard:           lboard,
		leaderboardOutputPath: lboardOutputPath,
		leaderboardIngress:    lboardIngress,
	}
	bot.OnMessage(t.onMsg)
	bot.OnPrivMessage(t.onPrivMsg)

	if err = t.generateLeaderboardPage(); err != nil {
		return nil, fmt.Errorf("failed to generate leaderboard page on startup: %w", err)
	}

	return t, nil
}

func (t *TriviaBot) Run() error {
	return t.bot.Run()
}

func (t *TriviaBot) onMsg(ctx context.Context, msg *bot.Msg) error {
	// TODO: implement a FlagSet that allows passing in of quiz properties
	// start: -duration, -category, -difficulty, -force

	if !strings.HasPrefix(msg.Data, "trivia") && !strings.HasPrefix(msg.Data, "!trivia") {
		return nil
	}

	if strings.Contains(msg.Data, "help") || strings.Contains(msg.Data, "info") {
		return t.bot.Send(
			"Start a new round with `trivia start`. Whisper me the number beside the answer `/w trivia 2`.",
		)
	}

	// TODO: when someone answer in public chat, send PM instructing user how to
	// properly answer
	// if t.quiz.InProgress {
	// }

	if strings.Contains(msg.Data, "leaderboard") || strings.Contains(msg.Data, "highscore") {
		return t.bot.Send(t.leaderboardIngress)
	}

	if strings.Contains(msg.Data, "start") || strings.Contains(msg.Data, "new") {
		if t.quiz != nil && t.quiz.InProgress() {
			return t.bot.Send("a quiz is already in progress")
		}

		fiveMinAgo := time.Now().Add(-5 * time.Minute)
		if t.lastQuizEndedAt.After(fiveMinAgo) {
			timeLeft := t.lastQuizEndedAt.Sub(fiveMinAgo).Round(time.Second)
			return t.bot.Send(fmt.Sprintf("on cooldown for %s PepoSleep", timeLeft))
		}

		// TODO: allow for providing quiz size
		quiz, err := trivia.NewDefaultQuiz(t.logger, t.source)
		if err != nil {
			return fmt.Errorf("failed to create a new quiz: %w", err)
		}
		t.quiz = quiz

		go func() {
			if err = t.runQuiz(ctx, msg.User); err != nil {
				t.logger.Fatalf("failed while running the quiz: %v", err)
			}
		}()
	}

	return nil
}

func (t *TriviaBot) onPrivMsg(ctx context.Context, msg *bot.Msg) error {
	t.logger.Debugw("private message received", "user", msg.User, "msg", msg.Data)

	if strings.HasPrefix(msg.Data, "remove") && msg.IsMod() {
		question := strings.TrimPrefix(msg.Data, "remove ")
		if question == "" {
			return t.bot.SendPriv("invalid question data", msg.User)
		}

		if err := t.setQuestionRemoved(ctx, question); err != nil {
			return t.bot.SendPriv(fmt.Sprintf("Error: %q", err), msg.User)
		}

		return t.bot.SendPriv("PepOk removed", msg.User)
	}

	if t.quiz != nil && t.quiz.InProgress() {
		answer, err := strconv.Atoi(msg.Data)
		if err != nil {
			return t.bot.SendPriv(
				"Invalid answer NOPERS whisper the number of the answer. `/w trivia 2`",
				msg.User,
			)
		}

		if !t.quiz.CurrentRound().NewParticipant(msg.User, answer-1, msg.Time) {
			return t.bot.SendPriv("Your answer is invalid or you have already submitted one!", msg.User)
		}

		return t.bot.SendPriv("Your answer has been locked in", msg.User)
	}

	return nil
}

func (t *TriviaBot) runQuiz(ctx context.Context, user string) error {
	if t.quiz.InProgress() {
		return errors.New("quiz is already in progress")
	}

	// insert who started the quiz to deter starting and not participating
	t.quiz.Scoreboard[user] = 0
	round, err := t.quiz.StartRound(t.onRoundCompletion)
	if err != nil {
		return fmt.Errorf("failed to start the round: %w", err)
	}

	t.logger.Infof("quiz started by %s", user)
	output := "Quiz starting soon! `/w trivia <number>` to answer."
	if err = t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send starting message: %w", err)
	}

	time.Sleep(10 * time.Second)

	if err = t.runRound(ctx, round); err != nil {
		return fmt.Errorf("error running round: %w", err)
	}

	// continue running rounds
	for !round.Final {
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

	output = "Quiz complete! The following users are awarded points: "
	if len(t.quiz.Scoreboard) == 0 {
		output += "No one! DuckerZ"
	} else {
		ss := t.quiz.Score()
		winners := []string{}
		for name, points := range ss {
			if points > 0 {
				winners = append(winners, fmt.Sprintf("%s +%d point(s)", name, points))
			}
		}

		if len(winners) == 0 {
			output += "No one! DuckerZ"
		} else {
			output += english.OxfordWordSeries(winners, "and")
		}

		if err = t.leaderboard.Update(ss); err != nil {
			return fmt.Errorf("failed to update leaderboard: %w", err)
		}
		if err = t.generateLeaderboardPage(); err != nil {
			return fmt.Errorf("failed to generated leaderboard: %w", err)
		}
	}

	return t.bot.Send(output)
}

func (t *TriviaBot) runRound(ctx context.Context, round *trivia.Round) error {
	leading := fmt.Sprintf("Round %d", round.Num)
	if round.Final {
		leading = "Final round"
	} else {
		defer func() {
			t.logger.Info("sleeping for 25 seconds until next round")
			time.Sleep(25 * time.Second)
		}()
	}

	output := leading + ": `" + strings.ReplaceAll(round.Question.Question, "`", "'") + "`"
	// answers have already been shuffled
	for idx, ans := range round.Question.Answers {
		output += fmt.Sprintf(" `%d) %s`", idx+1, ans.Value)
	}

	t.logger.Infow("running round and waiting for completion", "output", output)
	if err := t.bot.Send(output); err != nil {
		return fmt.Errorf("failed to send round start msgs: %w", err)
	}

	round.StartedAt = time.Now()

	for {
		if !t.quiz.InProgress() {
			t.logger.Info("round is no longer in progress.. breaking")
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

func (t *TriviaBot) onRoundCompletion(correct string, score []*trivia.Participant) error {
	output := fmt.Sprintf("Round complete! The correct answer is %s.", correct)
	defer func() {
		t.lastQuizEndedAt = time.Now()
		t.logger.Info(output)
	}()

	if len(score) == 0 {
		output += " No one answered correctly DuckerZ"
		return t.bot.Send(output)
	}

	var line string
	entries := []string{}
	for i := 0; i < len(score) && i <= 2; i++ {
		s := score[i]
		line = fmt.Sprintf("%s %s", humanize.Ordinal(i+1), s.Name)

		if i == 0 {
			rounded := s.TimeToSubmission.Round(time.Millisecond)
			line = fmt.Sprintf(" (%s to answer) %s", rounded, line)
		} else if i > 0 && len(score) >= 2 {
			diff := s.TimeToSubmission - score[i-1].TimeToSubmission
			line = fmt.Sprintf(" (+%s) %s", diff.Round(time.Millisecond), line)
		}

		entries = append(entries, line)
	}

	output += english.OxfordWordSeries(entries, "and")
	return t.bot.Send(output)
}

const tpl = `
<!DOCTYPE html>
<html>
  <head>
    <meta charset="UTF-8">
    <title>Strims Trivia Leaderboard</title>
  </head>
  <body>
    <h3>Season 3</h3>
    <small>Generated at {{ .TemplatedAt.Format "Jan 02, 2006 15:04:05 UTC" }}</small>
    <table>
      <tr>
        <th>Username</th>
        <th>Points</th>
        <th>Games</th>
        <th>Points/Game</th>
      </tr>
{{range .Highscores}}
      <tr>
        <td>{{.Name}}</td>
        <td>{{.Points}}</td>
        <td>{{.GamesPlayed}}</td>
        <td>{{divide .Points .GamesPlayed}}</td>
      </tr>
{{end}}
    </table>
    <h3>Season 2 top 10</h3>
    <table>
      <tr>
        <th>Username</th>
        <th>Points</th>
        <th>Games</th>
        <th>Points/Game</th>
      </tr>
      <tr>
        <td>tahley</td>
        <td>7753</td>
        <td>790</td>
        <td>9.813924</td>
      </tr>
      <tr>
        <td>salad</td>
        <td>5981</td>
        <td>748</td>
        <td>7.9959893</td>
      </tr>
      <tr>
        <td>Nhabls</td>
        <td>3659</td>
        <td>381</td>
        <td>9.603675</td>
      </tr>
      <tr>
        <td>bawrroccoli</td>
        <td>1149</td>
        <td>158</td>
        <td>7.272152</td>
      </tr>
      <tr>
        <td>mocha_z</td>
        <td>996</td>
        <td>147</td>
        <td>6.7755103</td>
      </tr>
      <tr>
        <td>laetus</td>
        <td>452</td>
        <td>86</td>
        <td>5.255814</td>
      </tr>
      <tr>
        <td>Gehirnchirurg</td>
        <td>435</td>
        <td>30</td>
        <td>14.5</td>
      </tr>
      <tr>
        <td>blankspaceblank</td>
        <td>371</td>
        <td>63</td>
        <td>5.888889</td>
      </tr>
      <tr>
        <td>haxzur</td>
        <td>347</td>
        <td>47</td>
        <td>7.382979</td>
      </tr>
      <tr>
        <td>rallen</td>
        <td>321</td>
        <td>37</td>
        <td>8.675675</td>
      </tr>
    </table>
    <hr class="solid">
    <h3>Season 1 top 10</h3>
    <table>
      <tr>
        <th>Username</th>
        <th>Points</th>
        <th>Games</th>
        <th>Points/Game</th>
      </tr>
      <tr>
        <td>tahley</td>
        <td>6451</td>
        <td>708</td>
        <td>9.111582</td>
      </tr>
      <tr>
        <td>anon</td>
        <td>5503</td>
        <td>837</td>
        <td>6.5746713</td>
      </tr>
      <tr>
        <td>salad</td>
        <td>4159</td>
        <td>589</td>
        <td>7.0611205</td>
      </tr>
      <tr>
        <td>Nhabls</td>
        <td>3273</td>
        <td>394</td>
        <td>8.307107</td>
      </tr>
      <tr>
        <td>bawrroccoli</td>
        <td>2045</td>
        <td>297</td>
        <td>6.885522</td>
      </tr>
      <tr>
        <td>guwap</td>
        <td>1861</td>
        <td>212</td>
        <td>8.778302</td>
      </tr>
      <tr>
        <td>blankspaceblank</td>
        <td>1753</td>
        <td>347</td>
        <td>5.051873</td>
      </tr>
      <tr>
        <td>mafaraxas</td>
        <td>1025</td>
        <td>173</td>
        <td>5.9248557</td>
      </tr>
      <tr>
        <td>KartoffelKopf</td>
        <td>831</td>
        <td>99</td>
        <td>8.393939</td>
      </tr>
      <tr>
        <td>Gehirnchirurg</td>
        <td>787</td>
        <td>56</td>
        <td>14.053572</td>
      </tr>
      </table>
  </body>
</html>`

func (t *TriviaBot) generateLeaderboardPage() error {
	highscores, err := t.leaderboard.Highscores(0)
	if err != nil {
		return fmt.Errorf("failed to get highscores: %w", err)
	}

	template, err := template.New("leaderboard").Funcs(template.FuncMap{
		"divide": func(a, b int64) string {
			return fmt.Sprintf("%g", float32(a)/float32(b))
		},
	}).Parse(tpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	templatedAt := time.Now()

	data := struct {
		TemplatedAt time.Time
		Highscores  []*models.User
	}{
		TemplatedAt: templatedAt,
		Highscores:  highscores,
	}

	if err = os.MkdirAll(filepath.Dir(t.leaderboardOutputPath), 0755); err != nil {
		return fmt.Errorf("failed to create leaderbord output dir (%s): %w", t.leaderboardOutputPath, err)
	}

	file, err := os.Create(t.leaderboardOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create leaderboard html file: %w", err)
	}

	if err = template.Execute(file, data); err != nil {
		return fmt.Errorf("failed to template page: %w", err)
	}

	if err = file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

func (t *TriviaBot) setQuestionRemoved(ctx context.Context, question string) error {
	q, err := models.Questions(models.QuestionWhere.Question.EQ(question)).OneG(ctx)
	if err != nil {
		return fmt.Errorf("failed to find question: %w", err)
	}
	q.Removed = "1"
	if _, err = q.UpdateG(ctx, boil.Infer()); err != nil {
		return fmt.Errorf("failed to update question in db: %w", err)
	}
	return nil
}
