// Package trivia ...
package trivia

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

type Source interface {
	Question() (*Question, error)
}

type Question struct {
	Question string
	Type     string
	Answers  []*Answer
}

type Answer struct {
	Value   string
	Correct bool
}

type Participant struct {
	Name             string
	Choice           int
	TimeToSubmission time.Duration
}

type Quiz struct {
	rw           sync.RWMutex
	logger       *zap.SugaredLogger
	duration     time.Duration
	currentRound int
	rng          *rand.Rand
	Rounds       []*Round
	Timer        *time.Timer
	inProgress   bool
	Scoreboard   map[string]int
}

func NewDefaultQuiz(logger *zap.SugaredLogger, sources ...Source) (*Quiz, error) {
	return NewQuiz(logger, 3, 30*time.Second, sources...)
}

func NewQuiz(logger *zap.SugaredLogger, size int, duration time.Duration, sources ...Source) (*Quiz, error) {
	quiz := &Quiz{
		duration:     duration,
		logger:       logger,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		currentRound: -1,
		Scoreboard:   map[string]int{},
	}

	quiz.logger.Info("creating new series of rounds")

	for i := 0; i < size; i++ {
		question, err := source.Question()
		if err != nil {
			return nil, err
		}

		quiz.Rounds = append(quiz.Rounds, &Round{
			logger:   logger,
			Question: question,
			Num:      i + 1,
			Final:    i == size-1,
		})
	}

	return quiz, nil
}

func (q *Quiz) CurrentRound() *Round {
	return q.Rounds[q.currentRound]
}

func (q *Quiz) InProgress() bool {
	q.rw.RLock()
	defer q.rw.RUnlock()
	return q.inProgress
}

func (q *Quiz) StartRound(
	onComplete func(string, []*Participant) error,
) (*Round, error) {

	if q.InProgress() {
		return nil, errors.New("a quiz is already in progress")
	}

	q.logger.Info("starting round")

	q.rw.Lock()
	q.inProgress = true
	q.currentRound++
	q.rw.Unlock()

	if q.currentRound >= len(q.Rounds) {
		return nil, errors.New("quiz is already complete")
	}
	round := q.Rounds[q.currentRound]
	question := round.Question

	q.logger.Infow("determined round...", "question", question)

	if question.Type == "boolean" {
		if len(question.Answers) != 2 {
			return nil, fmt.Errorf("unexpected answer count for boolean question %d", len(question.Answers))
		}
		if strings.ToLower(question.Answers[0].Value) != "true" {
			question.Answers[0], question.Answers[1] = question.Answers[1], question.Answers[0]
		}
	} else {
		q.rng.Shuffle(len(question.Answers), func(i, j int) {
			question.Answers[i], question.Answers[j] = question.Answers[j], question.Answers[i]
		})
	}

	q.Timer = time.AfterFunc(q.duration, func() {
		q.logger.Info("time is up!")
		q.rw.Lock()
		defer q.rw.Unlock()

		// append onto the current quiz leaderboard
		score := 3
		winners, losers := round.DetermineOutcome()
		for _, v := range winners {
			if score >= 1 {
				q.Scoreboard[v.Name] += score * 2
				score--
			} else {
				q.Scoreboard[v.Name] += 1
			}
		}

		for _, v := range losers {
			if _, ok := q.Scoreboard[v.Name]; !ok {
				q.Scoreboard[v.Name] = 0
			}
		}

		// determine correct answer and format it
		var correct string
		for idx, ans := range question.Answers {
			if ans.Correct {
				correct = fmt.Sprintf("`%d) %s`", idx+1, ans.Value)
				break
			}
		}

		q.logger.Infof("the correct answer is %q", correct)

		if err := onComplete(correct, winners); err != nil {
			q.logger.Fatalf("failed to run onComplete: %v", err)
		}

		q.inProgress = false
		round.Complete = true
	})

	q.logger.Infow("timer started, round set to in progress", "duration", q.duration)

	return round, nil
}

func (q *Quiz) Score() map[string]int {
	q.rw.RLock()
	defer q.rw.RUnlock()

	data := map[string]int{}
	for name, points := range q.Scoreboard {
		data[name] = points
	}

	return data
}

type Round struct {
	logger       *zap.SugaredLogger
	Question     *Question
	Participants []*Participant
	Complete     bool
	Num          int
	StartedAt    time.Time
	Final        bool
}

func (r *Round) NewParticipant(username string, answer int, timeIn int64) bool {
	for _, participant := range r.Participants {
		if participant.Name == username {
			return false
		}
	}

	if answer >= len(r.Question.Answers) {
		return false
	}

	timeToSub := time.Unix(timeIn/1000, timeIn%1000*int64(time.Millisecond)).Sub(r.StartedAt)
	p := &Participant{username, answer, timeToSub}

	r.Participants = append(r.Participants, p)
	r.logger.Infow("new participant", "entry", p)

	return true
}

func (r *Round) DetermineOutcome() ([]*Participant, []*Participant) {
	correctIdx := 0
	for idx, ans := range r.Question.Answers {
		if ans.Correct {
			correctIdx = idx
			break
		}
	}

	losers := []*Participant{}
	winners := []*Participant{}
	// filter participants for correct choice
	for _, participant := range r.Participants {
		if participant.Choice == correctIdx {
			winners = append(winners, participant)
		} else {
			losers = append(losers, participant)
		}
	}

	// sort participants by time in
	sort.Slice(winners, func(i, j int) bool {
		return winners[i].TimeToSubmission < winners[j].TimeToSubmission
	})

	r.logger.Infow("winners determined", "winners", winners)
	return winners, losers
}
