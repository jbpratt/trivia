package trivia

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/jbpratt/bots/internal/trivia/models"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"go.uber.org/zap"

	_ "github.com/mattn/go-sqlite3"
)

const sqlUserTable = `
/*
  Store users who have played a game of trivia and their total points
  over the history of playing. Sorting this table by points allows
  for determining an overall leaderboard.
*/
CREATE TABLE IF NOT EXISTS users (
  id           INTEGER NOT NULL PRIMARY KEY,
  name         TEXT    NOT NULL,
  points       INTEGER NOT NULL,
  games_played INTEGER NOT NULL
);
`

type Leaderboard struct {
	logger *zap.SugaredLogger
	db     *sql.DB
	rw     sync.RWMutex
}

func NewLeaderboard(logger *zap.SugaredLogger, db *sql.DB) (*Leaderboard, error) {
	if _, err := db.ExecContext(context.Background(), sqlUserTable); err != nil {
		return nil, fmt.Errorf("failed to run init sql: %w", err)
	}
	return &Leaderboard{
		logger: logger,
		db:     db,
	}, nil
}

func (l *Leaderboard) Update(entries map[string]int) error {
	l.rw.Lock()
	defer l.rw.Unlock()

	l.logger.Info("updating leaderboard with entries: %v", entries)

	ctx := context.Background()
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	for name, points := range entries {
		var user *models.User
		var exists bool

		exists, err = models.Users(models.UserWhere.Name.EQ(name)).ExistsG(ctx)
		if err != nil {
			return fmt.Errorf("failed to determine if user exists: %w", err)
		}

		if exists {
			user, err = models.Users(models.UserWhere.Name.EQ(name)).OneG(ctx)
			if err != nil {
				return fmt.Errorf("failed to get user(%s): %w", name, err)
			}

			l.logger.Infof("found user to update: %v", user)

			user.Points += int64(points)
			user.GamesPlayed++
			if _, err = user.UpdateG(ctx, boil.Infer()); err != nil {
				return fmt.Errorf("failed to update user: %w", err)
			}
		} else {
			user = &models.User{
				Name:        name,
				Points:      int64(points),
				GamesPlayed: 1,
			}
			l.logger.Infof("inserting new user: %v", user)
			if err = user.InsertG(ctx, boil.Infer()); err != nil {
				return fmt.Errorf("failed to insert new user: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (l *Leaderboard) Highscores(limit int) (models.UserSlice, error) {
	l.rw.RLock()
	defer l.rw.RUnlock()

	ctx := context.Background()
	if limit == 0 {
		size, err := models.Users().CountG(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get count of users: %w", err)
		}
		limit = int(size)
	}

	return models.Users(
		models.UserWhere.GamesPlayed.GT(0),
		qm.Select(models.UserColumns.Name, models.UserColumns.Points, models.UserColumns.GamesPlayed),
		qm.OrderBy("points desc"),
		qm.Limit(limit),
	).AllG(ctx)
}
