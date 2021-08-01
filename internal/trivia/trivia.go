// Package trivia ...
package trivia

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"time"

	"go.uber.org/zap"
)

const baseURL = "https://opentdb.com/"

type response struct {
	ResponseCode int `json:"response_code"`
	Results      []struct {
		Category         string   `json:"category"`
		Type             string   `json:"type"`
		Difficulty       string   `json:"difficulty"`
		Question         string   `json:"question"`
		CorrectAnswer    string   `json:"correct_answer"`
		IncorrectAnswers []string `json:"incorrect_answers"`
	} `json:"results"`
}

type Round struct {
	logger       *zap.SugaredLogger
	Category     string
	Difficulty   string
	Question     string
	Answers      []*Answer
	Participants []*Participant
	Complete     bool
	Num          int
	PrevRound    *Round
	NextRound    *Round
}

type Answer struct {
	Value   string
	Correct bool
}

type Participant struct {
	Name   string
	Choice int
	TimeIn int64
}

type Quiz struct {
	logger       *zap.SugaredLogger
	client       *http.Client
	url          string
	duration     time.Duration
	FirstRound   *Round
	CurrentRound *Round
	Timer        *time.Timer
	InProgress   bool
	Score        []Score
}

type Score struct {
	Points int
	Name   string
}

func NewDefaultQuiz(logger *zap.SugaredLogger) (*Quiz, error) {
	return NewQuiz(logger, 3, 30*time.Second)
}

func NewQuiz(logger *zap.SugaredLogger, size int, duration time.Duration) (*Quiz, error) {
	rand.Seed(time.Now().UnixNano())

	client := &http.Client{}
	resp, err := client.Get(fmt.Sprintf("%s/api_token.php?command=request", baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to request token: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.Debug("server returned bad response", "status_code", resp.StatusCode)
		return nil, fmt.Errorf("server returned invalid http code: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response body: %v", err)
	}

	tokenRes := struct {
		Token string `json:"token"`
	}{}

	if err = json.Unmarshal(body, &tokenRes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %v", err)
	}

	u, err := url.Parse(fmt.Sprintf("%s/api.php", baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %v", err)
	}

	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to parse query: %v", err)
	}

	q.Add("token", tokenRes.Token)
	q.Add("amount", fmt.Sprint(size))
	u.RawQuery = q.Encode()

	quiz := &Quiz{
		client:     client,
		url:        u.String(),
		duration:   duration,
		logger:     logger,
		InProgress: false,
		Score:      []Score{},
	}

	quiz.logger.Info("new quiz created, creating new series of rounds")
	if err = quiz.newSeries(); err != nil {
		return nil, fmt.Errorf("error creating new quiz: %v", err)
	}

	return quiz, nil
}

func (q *Quiz) newSeries() error {
	resp, err := q.client.Get(q.url)
	if err != nil {
		return fmt.Errorf("failed to get api data: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		q.logger.Debugw("bad response from server", "response_code", resp.StatusCode)
		return fmt.Errorf("server returned bad http status: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read api response body: %v", err)
	}

	var resultsResp response
	if err = json.Unmarshal(body, &resultsResp); err != nil {
		return fmt.Errorf("failed to unmarshal api response body: %v", err)
	}

	if len(resultsResp.Results) == 0 {
		return fmt.Errorf("server returned no results: %v", resultsResp)
	}

	roundNum := 1
	var head *Round
	var curr *Round
	for _, result := range resultsResp.Results {
		round := &Round{
			logger:     q.logger,
			Category:   result.Category,
			Difficulty: result.Difficulty,
			Question:   result.Question,
			Answers: []*Answer{
				{result.CorrectAnswer, true},
			},
			Num:       roundNum,
			PrevRound: curr,
			NextRound: nil,
			Complete:  false,
		}

		for _, value := range result.IncorrectAnswers {
			round.Answers = append(round.Answers, &Answer{value, false})
		}

		if head == nil {
			head = round
		}

		if curr != nil {
			curr.NextRound = round
		}

		curr = round
		roundNum++
	}

	q.FirstRound = head

	return nil
}

func (q *Quiz) StartRound(
	onComplete func(string, []*Participant) error,
) (*Round, error) {
	q.logger.Info("starting round")

	if q.FirstRound == nil {
		return nil, fmt.Errorf("rounds are not initialized")
	}

	// find the first round in the list that is not marked as complete
	q.CurrentRound = q.FirstRound
	for q.CurrentRound.Complete {
		if tmp := q.CurrentRound; tmp.NextRound != nil {
			q.CurrentRound = tmp.NextRound
		}
	}

	q.logger.Infow("determined round.. shuffling answers", "question", q.CurrentRound.Question)

	rand.Shuffle(len(q.CurrentRound.Answers), func(i, j int) {
		q.CurrentRound.Answers[i], q.CurrentRound.Answers[j] = q.CurrentRound.Answers[j], q.CurrentRound.Answers[i]
	})

	q.Timer = time.AfterFunc(q.duration, func() {
		q.logger.Info("time is up!")

		// append onto the current quiz leaderboard
		score := 3
		roundScore := q.CurrentRound.DetermineWinners()
		for _, v := range roundScore {
			if score == 0 {
				break
			}

			q.Score = append(q.Score, Score{score, v.Name})
			score--
		}

		// determine correct answer and format it
		var correct string
		for idx, ans := range q.CurrentRound.Answers {
			if ans.Correct {
				correct = fmt.Sprintf("`%d) %s`", idx+1, ans.Value)
				break
			}
		}

		q.logger.Infof("the correct answer is %q", correct)

		if err := onComplete(correct, roundScore); err != nil {
			q.logger.Fatalf("failed to run onComplete: %v", err)
		}
		q.InProgress = false
		q.CurrentRound.Complete = true
	})

	q.logger.Infow("timer started, round set to in progress", "duration", q.duration)
	q.InProgress = true

	return q.CurrentRound, nil
}

func (r *Round) NewParticipant(username string, answer int, time int64) bool {
	for _, participant := range r.Participants {
		if participant.Name == username {
			return false
		}
	}

	r.logger.Infow("new participant", "username", username, "answer", answer, "time", time)
	r.Participants = append(r.Participants, &Participant{username, answer, time})
	return true
}

func (r *Round) DetermineWinners() []*Participant {
	correctIdx := 0
	for idx, ans := range r.Answers {
		if ans.Correct {
			correctIdx = idx
			break
		}
	}

	winners := []*Participant{}
	// filter participants for correct choice
	for _, participant := range r.Participants {
		if participant.Choice == correctIdx {
			winners = append(winners, participant)
		}
	}

	// sort participants by time in
	sort.Slice(winners, func(i, j int) bool {
		return winners[i].TimeIn < winners[j].TimeIn
	})

	r.logger.Infow("winners determined", "winners", winners)
	return winners
}
