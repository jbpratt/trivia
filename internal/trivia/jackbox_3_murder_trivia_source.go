package trivia

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"time"
)

//go:embed json/jackbox-3-murder-trivia.json
var jackbox3MurderTriviaQuestionsJSON []byte

type Jackbox3MurderTriviaQuestion struct {
	Answer   string   `json:"answer"`
	Choices  []string `json:"options"`
	Question string   `json:"question"`
}

type Jackbox3MurderTriviaJSONSource struct {
	index     int
	Questions []*Jackbox3MurderTriviaQuestion
}

func NewJackbox3MurderTriviaJSONSource() (*Jackbox3MurderTriviaJSONSource, error) {
	var questions []*Jackbox3MurderTriviaQuestion
	if err := json.Unmarshal(jackbox3MurderTriviaQuestionsJSON, &questions); err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})

	src := &Jackbox3MurderTriviaJSONSource{
		index:     -1,
		Questions: questions,
	}
	return src, nil
}

func (s *Jackbox3MurderTriviaJSONSource) Question() (*Question, error) {
	s.index = (s.index + 1) % len(s.Questions)
	sq := s.Questions[s.index]

	q := &Question{
		Question: sq.Question,
	}

	for _, a := range sq.Choices {
		q.Answers = append(q.Answers, &Answer{
			Value:   a,
			Correct: a == sq.Answer,
		})
	}

	return q, nil
}
