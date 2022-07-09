package trivia

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/jbpratt/bots/internal/trivia/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

//go:embed sql/questions.sql
var sqlQuestionsEntries string

const sqlShuffleQuestsions = `
WITH t AS (
  SELECT id, row_number() OVER (ORDER BY random()) + n AS next_question_number
  FROM questions
  INNER JOIN question_sequence
  WHERE question_number = 0 OR question_number > n
)
UPDATE questions
SET question_number = (SELECT t.next_question_number FROM t WHERE t.id = questions.id)
WHERE id IN (SELECT id FROM t);
`

const sqlQuestionTable = `
/*
  Store trivia questions scraped from external sources. choices is a comma
  delimited list of all answers. Unique question allows for INSERT OR IGNORE.
*/
CREATE TABLE IF NOT EXISTS questions (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  question_number INTEGER NOT NULL DEFAULT 0,
  question        TEXT    NOT NULL,
  answer          TEXT    NOT NULL,
  choices         TEXT    NOT NULL,
  source          TEXT    NOT NULL,
  type            TEXT,
  removed         TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE(question)
);

CREATE TABLE IF NOT EXISTS question_sequence (
  n INTEGER PRIMARY KEY NOT NULL
);
`

type DBSource struct {
	cache []*Question
	db    *sql.DB
}

func NewDefaultDBSource(db *sql.DB) (*DBSource, error) {
	return NewDBSource(db)
}

func NewDBSource(db *sql.DB) (*DBSource, error) {
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, sqlQuestionTable+sqlQuestionsEntries); err != nil {
		return nil, fmt.Errorf("failed to run init sql: %w", err)
	}

	count, err := models.QuestionSequences().CountG(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get count of question_sequence table: %w", err)
	}

	if count == 0 {
		if err = (&models.QuestionSequence{N: 0}).InsertG(ctx, boil.Infer()); err != nil {
			return nil, fmt.Errorf("failed to insert starting sequence: %w", err)
		}

		if _, err = db.ExecContext(ctx, sqlShuffleQuestsions); err != nil {
			return nil, fmt.Errorf("failed to run shuffle questions sql: %w", err)
		}
	}

	return &DBSource{db: db}, nil
}

func (s *DBSource) refreshCache(ctx context.Context) error {
	sequence, err := models.QuestionSequences().OneG(ctx)
	if err != nil {
		return fmt.Errorf("failed to query question sequence: %w", err)
	}

	questions, err := models.Questions(
		qm.Where("question_number > ?", sequence.N),
		qm.OrderBy("question_number asc"),
		qm.Limit(3),
	).AllG(ctx)
	if err != nil {
		return fmt.Errorf("failed to query questions: %w", err)
	}

	for _, question := range questions {
		choices := strings.Split(question.Choices, ",")
		q := &Question{
			Question: question.Question,
			Answers:  []*Answer{},
		}

		for _, choice := range choices {
			q.Answers = append(q.Answers, &Answer{
				Value:   choice,
				Correct: choice == question.Answer,
			})
		}

		s.cache = append(s.cache, q)
	}

	if _, err = s.db.ExecContext(ctx, "UPDATE question_sequence SET n = n + ?", 3); err != nil {
		return fmt.Errorf("failed to increment question sequence: %w", err)
	}

	return nil
}

func (s *DBSource) Question() (*Question, error) {
	if len(s.cache) == 0 {
		if err := s.refreshCache(context.Background()); err != nil {
			return nil, err
		}
	}

	l := len(s.cache) - 1
	q := s.cache[l]
	s.cache = s.cache[:l]

	return q, nil
}
