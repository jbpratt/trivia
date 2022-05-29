package trivia

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"time"
)

//go:embed json/hq-trivia.json
var hqTriviaQuestionsJSON []byte

type HQTriviaQuestion struct {
	Question string `json:"question"`
	Choices  []struct {
		Text    string `json:"text"`
		Correct bool   `json:"correct"`
	} `json:"answers"`
}

type HQTriviaJSONSource struct {
	index     int
	Questions []*HQTriviaQuestion
}

func NewHQTriviaJSONSource() (*HQTriviaJSONSource, error) {
	var questions []*HQTriviaQuestion
	if err := json.Unmarshal(hqTriviaQuestionsJSON, &questions); err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})

	src := &HQTriviaJSONSource{
		index:     -1,
		Questions: questions,
	}
	return src, nil
}

func (s *HQTriviaJSONSource) Question() (*Question, error) {
	s.index = (s.index + 1) % len(s.Questions)
	sq := s.Questions[s.index]

	q := &Question{
		Question: sq.Question,
	}

	for _, a := range sq.Choices {
		q.Answers = append(q.Answers, &Answer{
			Value:   a.Text,
			Correct: a.Correct,
		})
	}

	return q, nil
}
