package trivia

import (
	"encoding/json"
	"math/rand"
	"os"
	"time"
)

type MillionaireDBQuestion struct {
	Answer   string   `json:"answer"`
	Choices  []string `json:"choices"`
	Question string   `json:"question"`
}

type MillionaireDBJSONSource struct {
	index     int
	Questions []*MillionaireDBQuestion
}

func NewMillionaireDBJSONSource(path string) (*MillionaireDBJSONSource, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0400)
	if err != nil {
		return nil, err
	}

	var questions []*MillionaireDBQuestion
	if err := json.NewDecoder(f).Decode(&questions); err != nil {
		return nil, err
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})

	src := &MillionaireDBJSONSource{
		index:     -1,
		Questions: questions,
	}
	return src, nil
}

func (s *MillionaireDBJSONSource) Question() (*Question, error) {
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
