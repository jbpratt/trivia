package trivia_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jbpratt/bots/internal/trivia"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

type testSource struct {
	currentQuestion int
	questions       []*trivia.Question
	err             error
}

func (t *testSource) Question(_ context.Context) (*trivia.Question, error) {
	if t.err != nil {
		return nil, t.err
	}
	question := t.questions[t.currentQuestion]
	t.currentQuestion++
	return question, nil
}

func TestQuiz(t *testing.T) {
	source := &testSource{
		questions: []*trivia.Question{
			{
				Question: "Question 0",
				Answers: []*trivia.Answer{
					{Value: "Correct", Correct: true},
					{Value: "Wrong0", Correct: false},
					{Value: "Wrong1", Correct: false},
				},
			},
			{
				Question: "Question 1",
				Answers: []*trivia.Answer{
					{Value: "Correct", Correct: true},
					{Value: "Wrong", Correct: false},
				},
			},
			{
				Question: "Question 2",
				Answers: []*trivia.Answer{
					{Value: "Correct", Correct: true},
					{Value: "Wrong", Correct: false},
				},
			},
			{
				Type:     "boolean",
				Question: "Boolean test question?",
				Answers: []*trivia.Answer{
					{Value: "true", Correct: true},
					{Value: "false", Correct: false},
				},
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()

	quiz, err := trivia.NewQuiz(t.Context(), logger, 3, 1*time.Second, source)
	require.NoError(t, err)

	var roundsComplete int
	onComplete := func(_ context.Context, answer string, winners []*trivia.Participant) error {
		roundsComplete++
		return nil
	}

	for range 3 {
		round, err := quiz.StartRound(t.Context(), onComplete)
		require.NoError(t, err)

		for j := range 2 {
			require.True(t, round.NewParticipant(fmt.Sprintf("User%d", j), j, time.Now().UnixNano()))
			require.False(t, round.NewParticipant(fmt.Sprintf("User%d", j), j, time.Now().UnixNano()))
		}

		require.False(t, round.NewParticipant("NewUser", 9, time.Now().UnixNano()))

		for quiz.InProgress() {
			time.Sleep(49 * time.Millisecond)
		}
	}

	require.Equal(t, 3, roundsComplete)

	_, err = quiz.StartRound(t.Context(), onComplete)
	require.Error(t, err)

	score := quiz.Score()
	require.Len(t, score, 2)
}

func TestQuizCancellation(t *testing.T) {
	source := &testSource{
		questions: []*trivia.Question{
			{
				Question: "Test Question",
				Answers: []*trivia.Answer{
					{Value: "Answer 1", Correct: true},
					{Value: "Answer 2", Correct: false},
				},
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	quiz, err := trivia.NewQuiz(t.Context(), logger, 1, 10*time.Second, source)
	require.NoError(t, err)

	onComplete := func(ctx context.Context, answer string, winners []*trivia.Participant) error {
		return nil
	}

	err = quiz.Cancel()
	require.Error(t, err)

	_, err = quiz.StartRound(t.Context(), onComplete)
	require.NoError(t, err)

	require.True(t, quiz.InProgress())

	err = quiz.Cancel()
	require.NoError(t, err)

	require.False(t, quiz.InProgress())

	select {
	case <-quiz.Done():
		// Expected, context should be canceled
	default:
		t.Fatal("Quiz context was not canceled")
	}
}
