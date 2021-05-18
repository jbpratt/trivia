package trivia

import (
	"errors"
	"fmt"

	"go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var bucketName = []byte("leaderboard")

type Leaderboard struct {
	logger *zap.SugaredLogger
	db     *bbolt.DB
}

func NewLeaderboard(logger *zap.SugaredLogger, path string) (*Leaderboard, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %v", err)
	}

	tx, err := db.Begin(true)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %v", err)
	}

	_, err = tx.CreateBucketIfNotExists(bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket: %v", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &Leaderboard{logger, db}, nil
}

func (l *Leaderboard) Update(data map[string]int) error {
	return l.db.Batch(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		for k, v := range data {
			if err := b.Put([]byte(k), []byte(fmt.Sprint(v))); err != nil {
				return fmt.Errorf("failed to insert: %v/%v", k, v)
			}
		}
		return nil
	})
}

func (l *Leaderboard) Get(limit int) (map[string]int, error) {
	i := 0
	output := make(map[string]int)
	return output, l.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketName)
		return b.ForEach(func(k, v []byte) error {
			if i == limit {
				return errors.New("limit reached")
			}
			output[string(k)] = 0
			i++
			return nil
		})
	})
}
