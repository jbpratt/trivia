// Package trivia ...
package trivia

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jbpratt/bots/internal/trivia/models"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

//go:embed init.sql
var initSql string

//go:embed questions.sql
var questionsSql string

type DBSource struct {
	cacheSize int
	cache     []*Question
	db        *sql.DB
}

func NewDefaultDBSource(db *sql.DB) (*DBSource, error) {
	return NewDBSource(db, 15)
}

func NewDBSource(db *sql.DB, cacheSize int) (*DBSource, error) {
	if _, err := db.ExecContext(context.Background(), initSql+questionsSql); err != nil {
		return nil, fmt.Errorf("failed to run init sql: %w", err)
	}
	return &DBSource{
		cacheSize: cacheSize,
		db:        db,
	}, nil
}

func (s *DBSource) refreshCache() error {
	questions, err := models.Questions(
		qm.OrderBy("used desc"),
		qm.Limit(s.cacheSize),
	).AllG(context.Background())
	if err != nil {
		return fmt.Errorf("failed to query questions: %w", err)
	}

	for _, question := range questions {
		answers := []*Answer{}
		rawAnswers := strings.Split(question.Choices, ",")
		for _, a := range rawAnswers {
			answers = append(answers, &Answer{
				Value:   a,
				Correct: a == question.Answer,
			})
		}

		s.cache = append(s.cache, &Question{
			Question: question.Question,
			Answers:  answers,
		})
	}

	// mark the question as used

	return nil
}

func (s *DBSource) Question() (*Question, error) {
	if len(s.cache) == 0 {
		if err := s.refreshCache(); err != nil {
			return nil, err
		}
	}

	l := len(s.cache) - 1
	q := s.cache[l]
	s.cache = s.cache[:l]

	return q, nil
}
